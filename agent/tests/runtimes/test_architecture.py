"""T14 运行时目录的静态依赖和实现边界测试。"""

import ast
from pathlib import Path
from typing import Final

RUNTIME_ROOT: Final = Path(__file__).parents[2] / "src" / "scyg_agent" / "runtimes"
BOUNDARY_FILES: Final = ("base.py", "registry.py", "router.py")
FORBIDDEN_IMPORTS: Final = (
    "importlib",
    "pkg_resources",
    "langgraph",
    "deepagents",
    "openai",
    "fastapi",
    "grpc",
    "sqlalchemy",
)
FORBIDDEN_CALLS: Final = (
    "__import__",
    "entry_points",
    "find_spec",
    "import_module",
    "module_from_spec",
)
FORBIDDEN_TYPING: Final = ("Any", "cast", "object")


def _qualified_name(node: ast.expr) -> str:
    """提取调用目标末段名称供架构规则比较。"""
    if isinstance(node, ast.Name):
        return node.id
    if isinstance(node, ast.Attribute):
        return node.attr
    return ""


def test_registry_boundary_has_no_framework_or_dynamic_loading_imports() -> None:
    # Given: 注册表、路由和适配器协议三个纯边界模块。
    violations: list[str] = []
    for filename in BOUNDARY_FILES:
        path = RUNTIME_ROOT / filename
        tree = ast.parse(path.read_text(encoding="utf-8"), filename=str(path))

        # When: 检查导入、动态模块调用和禁止的类型逃逸口。
        for node in ast.walk(tree):
            if isinstance(node, ast.Import):
                violations.extend(
                    f"{filename}:import:{alias.name}"
                    for alias in node.names
                    if alias.name.startswith(FORBIDDEN_IMPORTS)
                )
            if (
                isinstance(node, ast.ImportFrom)
                and node.module is not None
                and node.module.startswith(FORBIDDEN_IMPORTS)
            ):
                violations.append(f"{filename}:from:{node.module}")
            if isinstance(node, ast.Call) and _qualified_name(node.func) in FORBIDDEN_CALLS:
                violations.append(f"{filename}:call:{_qualified_name(node.func)}")
            if isinstance(node, ast.Name) and node.id in FORBIDDEN_TYPING:
                violations.append(f"{filename}:typing:{node.id}")

    # Then: 目录保持标准库和应用类型所有权, 不发现插件或框架耦合.
    assert violations == []


def test_registry_boundary_has_no_mutable_module_registry_or_broad_exception() -> None:
    # Given: 所有 T14 运行时源模块。
    violations: list[str] = []
    for path in sorted(RUNTIME_ROOT.glob("*.py")):
        tree = ast.parse(path.read_text(encoding="utf-8"), filename=str(path))

        # When: 检查模块级可变容器和宽泛异常处理。
        for node in tree.body:
            if isinstance(node, (ast.Assign, ast.AnnAssign)):
                value = node.value
                if isinstance(value, (ast.Dict, ast.List, ast.Set)):
                    violations.append(f"{path.name}:mutable-global")
        violations.extend(
            f"{path.name}:broad-except:{node.type.id}"
            for node in ast.walk(tree)
            if isinstance(node, ast.ExceptHandler)
            and isinstance(node.type, ast.Name)
            and node.type.id in {"BaseException", "Exception"}
        )

    # Then: 构造完全由冻结值和显式条目驱动。
    assert violations == []
