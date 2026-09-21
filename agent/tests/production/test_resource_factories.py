"""认证与 SIMPLE 生产资源工厂契约。"""

from pathlib import Path

import httpx
import pytest
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import ec, rsa
from pydantic import SecretStr

from scyg_agent.adapters.auth.factory import (
    InvalidJwtPublicKeyError,
    JwtPemSource,
    create_jwt_verifier,
    load_jwt_rsa_public_key,
)
from scyg_agent.config import load_settings
from scyg_agent.runtimes.simple.factory import open_simple_provider


@pytest.fixture
def anyio_backend() -> str:
    return "asyncio"


def _public_pem() -> bytes:
    key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    return key.public_key().public_bytes(
        serialization.Encoding.PEM,
        serialization.PublicFormat.SubjectPublicKeyInfo,
    )


def test_jwt_pem_loader_accepts_exact_rsa_and_redacts_failures(tmp_path: Path) -> None:
    valid = tmp_path / "public.pem"
    invalid = tmp_path / "private-secret-name.pem"
    _ = valid.write_bytes(_public_pem())
    _ = invalid.write_text("not-a-key")

    assert isinstance(load_jwt_rsa_public_key(JwtPemSource(path=valid)), rsa.RSAPublicKey)
    with pytest.raises(InvalidJwtPublicKeyError) as captured:
        _ = load_jwt_rsa_public_key(JwtPemSource(path=invalid))
    assert "private-secret-name" not in str(captured.value)
    assert "not-a-key" not in repr(captured.value)


def test_jwt_pem_loader_rejects_non_rsa_secret_without_repr_leak() -> None:
    key = ec.generate_private_key(ec.SECP256R1()).public_key()
    pem = key.public_bytes(
        serialization.Encoding.PEM, serialization.PublicFormat.SubjectPublicKeyInfo
    )
    source = JwtPemSource(secret=SecretStr(pem.decode()))

    with pytest.raises(InvalidJwtPublicKeyError):
        _ = load_jwt_rsa_public_key(source)
    assert pem.decode() not in repr(source)


def test_create_jwt_verifier_reads_frozen_settings(
    configured_environment: None, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    _ = configured_environment
    path = tmp_path / "jwt.pem"
    _ = path.write_bytes(_public_pem())
    monkeypatch.setenv("SCYG_AGENT_JWT_PUBLIC_KEY_PATH", str(path))

    verifier = create_jwt_verifier(load_settings())

    assert verifier.policy.issuer == "scyg-blog"


@pytest.mark.anyio
async def test_simple_provider_borrows_injected_client_and_factory_owns_internal(
    configured_environment: None,
) -> None:
    _ = configured_environment
    settings = load_settings()
    borrowed = httpx.AsyncClient(
        transport=httpx.MockTransport(lambda _request: httpx.Response(200)),
        base_url="https://provider.test/v1",
    )

    async with open_simple_provider(settings, client=borrowed) as provider:
        await provider.aclose()
    assert not borrowed.is_closed
    await borrowed.aclose()

    async with open_simple_provider(settings):
        pass
