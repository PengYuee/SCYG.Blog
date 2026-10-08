"""Deterministic Python protobuf generation and drift checking."""

from __future__ import annotations

import ast
import hashlib
import os
import shutil
import subprocess
import sys
import tempfile
import uuid
from dataclasses import dataclass
from pathlib import Path
from typing import TYPE_CHECKING, Final, final, override

if TYPE_CHECKING:
    from collections.abc import Mapping, Sequence

BUF_PACKAGE: Final = "@bufbuild/buf@1.47.2"
PACKAGE_IMPORT: Final = "from scyg."
OWNED_PACKAGE_IMPORT: Final = "from scyg_agent.generated.proto.scyg."
VALIDATE_IMPORT: Final = "from buf."
OWNED_VALIDATE_IMPORT: Final = "from scyg_agent.generated.proto.buf."
PACKAGE_INITIALIZER: Final = '"""Generated protobuf package."""\n'
_GRPC_CALLABLES: Final = {
    "unary_unary": "UnaryUnaryMultiCallable",
    "unary_stream": "UnaryStreamMultiCallable",
}


@final
class ContractRepositoryNotFoundError(FileNotFoundError):
    """Require repository inputs while preserving the filesystem error category."""

    @override
    def __str__(self) -> str:
        """Return the stable generation prerequisite diagnostic."""
        return "run contract generation inside the SCYG repository"


@final
class UnsupportedGrpcBindingError(ValueError):
    """Reject generated binding shapes outside the owned unary RPC contract."""

    @override
    def __str__(self) -> str:
        """Report a generator incompatibility without inventing message types."""
        return "unsupported generated gRPC binding shape"


@dataclass(frozen=True, slots=True)
class _GrpcMethod:
    name: str
    channel_method: str
    request: str
    response: str


def _message_type(serializer: ast.expr) -> str:
    if not isinstance(serializer, ast.Attribute) or not isinstance(serializer.value, ast.Attribute):
        raise UnsupportedGrpcBindingError
    return ast.unparse(serializer.value)


def _rpc_assignment(statement: ast.stmt) -> _GrpcMethod | None:
    if not isinstance(statement, ast.Assign) or len(statement.targets) != 1:
        return None
    target, call = statement.targets[0], statement.value
    if not isinstance(target, ast.Attribute) or not isinstance(call, ast.Call):
        raise UnsupportedGrpcBindingError
    if not isinstance(call.func, ast.Attribute) or call.func.attr not in _GRPC_CALLABLES:
        raise UnsupportedGrpcBindingError
    parameters = {keyword.arg: keyword.value for keyword in call.keywords}
    request = parameters.get("request_serializer")
    response = parameters.get("response_deserializer")
    if request is None or response is None:
        raise UnsupportedGrpcBindingError
    return _GrpcMethod(target.attr, call.func.attr, _message_type(request), _message_type(response))


def _rpc_methods(stub: ast.ClassDef) -> tuple[_GrpcMethod, ...]:
    initializer = next(
        (
            statement
            for statement in stub.body
            if isinstance(statement, ast.FunctionDef) and statement.name == "__init__"
        ),
        None,
    )
    if initializer is None:
        raise UnsupportedGrpcBindingError
    methods = tuple(
        method
        for statement in initializer.body
        if (method := _rpc_assignment(statement)) is not None
    )
    if not methods:
        raise UnsupportedGrpcBindingError
    return methods


def _grpc_declarations(source: str) -> str:
    """Derive aio service typing from protoc's actual serializer bindings."""
    tree = ast.parse(source)
    services = tuple(
        (node.name.removesuffix("Stub"), _rpc_methods(node))
        for node in tree.body
        if isinstance(node, ast.ClassDef) and node.name.endswith("Stub")
    )
    if not services:
        return "# Generated from service-less protoc gRPC bindings; do not edit.\n"
    asynchronous_types = sorted(
        {
            kind
            for _, methods in services
            for method in methods
            for kind in (
                ("AsyncIterator", "Iterator")
                if method.channel_method == "unary_stream"
                else ("Awaitable",)
            )
        }
    )
    lines = [
        "# Generated from protoc gRPC bindings; do not edit.",
        "from collections.abc import " + ", ".join(asynchronous_types),
        "from typing import Protocol",
        "",
        "from grpc import aio",
        "",
    ]
    lines.extend(
        ast.unparse(node)
        for node in tree.body
        if isinstance(node, ast.ImportFrom)
        and node.module is not None
        and node.module.startswith("scyg_agent.generated.proto.")
    )
    for service, methods in services:
        lines.extend(
            (
                "",
                f"class {service}Stub:",
                "    def __init__(self, channel: aio.Channel) -> None: ...",
            )
        )
        for method in methods:
            callable_name = _GRPC_CALLABLES[method.channel_method]
            call_type = f"aio.{callable_name}[{method.request}, {method.response}]"
            lines.append(f"    {method.name}: {call_type}")
        lines.extend(("", f"class {service}Servicer(Protocol):"))
        for method in methods:
            response = (
                f"Iterator[{method.response}] | AsyncIterator[{method.response}]"
                if method.channel_method == "unary_stream"
                else f"{method.response} | Awaitable[{method.response}]"
            )
            lines.extend(
                (
                    f"    def {method.name}(",
                    f"        self, request: {method.request},",
                    f"        context: aio.ServicerContext[{method.request}, {method.response}],",
                    f"    ) -> {response}: ...",
                )
            )
        lines.extend(
            (
                "",
                f"def add_{service}Servicer_to_server(",
                f"    servicer: {service}Servicer, server: aio.Server",
                ") -> None: ...",
            )
        )
    return "\n".join(lines) + "\n"


class GeneratedTreeRollbackError(OSError):
    """Report a failed generated-tree swap whose backup could not be restored."""

    # backup identifies the preserved old generated tree for manual recovery.
    backup: Path
    # swap_error is the original staged-to-target rename failure.
    swap_error: OSError
    # rollback_error is the subsequent backup-to-target rename failure.
    rollback_error: OSError

    def __init__(self, backup: Path, swap_error: OSError, rollback_error: OSError) -> None:
        """Preserve both filesystem failures and the recoverable backup path."""
        super().__init__(f"generated tree rollback failed; backup preserved at {backup}")
        self.backup = backup
        self.swap_error = swap_error
        self.rollback_error = rollback_error


@dataclass(frozen=True, slots=True)
class GeneratedTrees:
    """Paths participating in one isolated generation operation."""

    repository: Path
    expected: Path
    generated: Path


def repository_root() -> Path:
    """Locate repository inputs without relying on an editable installation."""
    for start in (Path.cwd().resolve(), Path(__file__).resolve().parent):
        for candidate in (start, *start.parents):
            if (candidate / "contracts" / "buf.yaml").is_file() and (
                candidate / "agent" / "pyproject.toml"
            ).is_file():
                return candidate
    raise ContractRepositoryNotFoundError


def generate_contracts(destination: Path) -> None:
    """Generate and package Python bindings in an isolated destination."""
    repository = repository_root()
    contracts = repository / "contracts"
    executable = os.environ.get("SCYG_BUF_BIN")
    npx = shutil.which("npx") if executable is None else None
    if executable is None and npx is None:
        _ = sys.stderr.write("npx or SCYG_BUF_BIN is required for pinned Buf generation\n")
        raise SystemExit(2)
    prefix = (executable,) if executable is not None else (str(npx), "--yes", BUF_PACKAGE)
    command = (
        *prefix,
        "generate",
        "--template",
        "buf.gen.yaml",
        "--output",
        str(destination),
        "--include-imports",
        "--timeout",
        "2m",
    )
    # The executable is PATH-resolved and every argument is repository-owned or constant.
    _ = subprocess.run(command, cwd=contracts, check=True, timeout=150)  # noqa: S603

    for path in sorted(path for path in destination.rglob("*") if path.suffix in {".py", ".pyi"}):
        source = path.read_text(encoding="utf-8")
        rewritten = source.replace(PACKAGE_IMPORT, OWNED_PACKAGE_IMPORT).replace(
            VALIDATE_IMPORT, OWNED_VALIDATE_IMPORT
        )
        _ = path.write_text(rewritten, encoding="utf-8", newline="\n")
        if path.name.endswith("_pb2_grpc.py"):
            _ = path.with_suffix(".pyi").write_text(
                _grpc_declarations(rewritten), encoding="utf-8", newline="\n"
            )
    packages = (destination, *sorted(path for path in destination.rglob("*") if path.is_dir()))
    for package in packages:
        _ = (package / "__init__.py").write_text(
            PACKAGE_INITIALIZER,
            encoding="utf-8",
            newline="\n",
        )


def _file_bytes(root: Path) -> Mapping[str, bytes]:
    """Read a generated tree as stable POSIX-relative byte entries."""
    return {
        path.relative_to(root).as_posix(): path.read_bytes()
        for path in sorted(root.rglob("*"))
        if path.is_file() and "__pycache__" not in path.parts and path.suffix != ".pyc"
    }


def _trees(temporary_root: Path) -> GeneratedTrees:
    """Resolve expected and fresh generated trees."""
    repository = repository_root()
    return GeneratedTrees(
        repository=repository,
        expected=repository / "agent" / "src" / "scyg_agent" / "generated" / "proto",
        generated=temporary_root,
    )


def report_drift(expected: Mapping[str, bytes], actual: Mapping[str, bytes]) -> int:
    """Report every missing, extra, or byte-changed generated file."""
    expected_paths = set(expected)
    actual_paths = set(actual)
    missing = sorted(actual_paths - expected_paths)
    extra = sorted(expected_paths - actual_paths)
    changed = sorted(
        path for path in expected_paths & actual_paths if expected[path] != actual[path]
    )
    for category, paths in (("missing", missing), ("extra", extra), ("changed", changed)):
        for path in paths:
            _ = sys.stderr.write(f"{category}: {path}\n")
    if missing or extra or changed:
        return 1
    _ = sys.stdout.write(f"generated contracts are current ({len(actual)} files)\n")
    return 0


def _check() -> int:
    """Generate fresh bindings and byte-compare them with the local generated tree."""
    with tempfile.TemporaryDirectory(prefix="scyg-contract-check-") as temporary:
        trees = _trees(Path(temporary))
        generate_contracts(Path(temporary))
        return report_drift(_file_bytes(trees.expected), _file_bytes(trees.generated))


def write_contracts() -> int:
    """Atomically replace the owned generated tree from fresh Buf output."""
    with tempfile.TemporaryDirectory(prefix="scyg-contract-generate-") as temporary:
        trees = _trees(Path(temporary))
        target = trees.expected
        generate_contracts(Path(temporary))
        target.parent.mkdir(parents=True, exist_ok=True)
        transaction_id = uuid.uuid4().hex
        staged = target.parent / f".generated-staged-{transaction_id}"
        backup = target.parent / f".generated-backup-{transaction_id}"
        _ = shutil.copytree(trees.generated, staged)
        target_existed = target.exists()
        backup_created = False
        commit_succeeded = False
        rollback_succeeded = False
        try:
            if target_existed:
                _ = target.rename(backup)
                backup_created = True
            try:
                _ = staged.rename(target)
                commit_succeeded = True
            except OSError as swap_error:
                if backup_created:
                    try:
                        _ = backup.rename(target)
                        rollback_succeeded = True
                    except OSError as rollback_error:
                        raise GeneratedTreeRollbackError(
                            backup,
                            swap_error,
                            rollback_error,
                        ) from rollback_error
                raise
        finally:
            if staged.exists():
                shutil.rmtree(staged)
            if backup.exists() and (commit_succeeded or rollback_succeeded):
                shutil.rmtree(backup)
        manifest = _file_bytes(target)
        for obsolete in (target.parent / "scyg", target.parent / "buf"):
            if obsolete.exists():
                shutil.rmtree(obsolete)
        aggregate = hashlib.sha256(
            b"".join(path.encode() + b"\0" + manifest[path] for path in sorted(manifest))
        ).hexdigest()
        _ = sys.stdout.write(f"generated {len(manifest)} files (sha256 {aggregate})\n")
    return 0


def run(arguments: Sequence[str]) -> int:
    """Run the bounded generation command."""
    normalized = tuple(arguments)
    if normalized == ("generate",):
        return write_contracts()
    if normalized == ("check",):
        return _check()
    _ = sys.stderr.write("usage: scyg-agent-contracts {generate|check}\n")
    return 2


def main() -> None:
    """Run contract generation using process arguments."""
    raise SystemExit(run(sys.argv[1:]))


if __name__ == "__main__":
    main()
