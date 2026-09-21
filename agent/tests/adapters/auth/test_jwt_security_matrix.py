"""JWT 结构与服务主体的补充安全矩阵."""

import base64
from datetime import UTC, datetime

import jwt
import pytest
from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.asymmetric import padding, rsa

from scyg_agent.adapters.auth import (
    AuthenticationError,
    AuthPolicy,
    BlogServicePrincipal,
    EncodedJwt,
    JwtVerifier,
)

NOW = 2_000_000_000


class FixedClock:
    """提供确定性时间边界."""

    def now(self) -> datetime:
        """返回固定 UTC 时刻."""
        return datetime.fromtimestamp(NOW, tz=UTC)


@pytest.fixture(scope="module")
def private_key() -> rsa.RSAPrivateKey:
    """创建仅驻留内存的测试签名密钥."""
    return rsa.generate_private_key(public_exponent=65537, key_size=2048)


@pytest.fixture
def verifier(private_key: rsa.RSAPrivateKey) -> JwtVerifier:
    """构造固定 Blog 服务认证策略."""
    return JwtVerifier(
        AuthPolicy(
            public_key=private_key.public_key(),
            issuer="blog-api",
            audience="agent-service",
            blog_service_subject="blog-api-service",
            clock_skew_seconds=30,
        ),
        FixedClock(),
    )


def blog_claims() -> dict[str, str | int | list[str]]:
    """返回合法 Blog 服务声明."""
    return {
        "iss": "blog-api",
        "aud": "agent-service",
        "sub": "blog-api-service",
        "exp": NOW + 300,
        "nbf": NOW,
        "iat": NOW,
        "scope": ["agent:runs:create", "agent:runs:read"],
        "principal_kind": "blog_service",
    }


def compact(private_key: rsa.RSAPrivateKey, header: bytes, payload: bytes) -> EncodedJwt:
    """签名手工 JSON 段以覆盖重复成员等编码边界."""

    def encode(value: bytes) -> bytes:
        """生成无填充 base64url 段."""
        return base64.urlsafe_b64encode(value).rstrip(b"=")

    signing_input = b".".join((encode(header), encode(payload)))
    signature = private_key.sign(signing_input, padding.PKCS1v15(), hashes.SHA256())
    return EncodedJwt(b".".join((signing_input, encode(signature))).decode("ascii"))


@pytest.mark.parametrize(
    "mutation",
    [
        {"sub": "other-service"},
        {"scope": ["agent:runs:create"]},
        {"scope": ["agent:runs:create", "agent:runs:read", "agent:runs:read"]},
        {"exp": NOW - 30},
        {"nbf": NOW + 31},
        {"iat": NOW + 31},
    ],
)
def test_blog_service_policy_rejects_subject_scope_and_time(
    verifier: JwtVerifier,
    private_key: rsa.RSAPrivateKey,
    mutation: dict[str, str | int | list[str]],
) -> None:
    # Given: Blog 服务声明违反配置主体、最小 scope 或偏差边界。
    payload = {**blog_claims(), **mutation}
    token = EncodedJwt(jwt.encode(payload, private_key, algorithm="RS256"))
    # When/Then: 所有策略失败均使用同一隐私安全消息。
    with pytest.raises(AuthenticationError, match="^身份令牌无效$"):
        _ = verifier.verify(token)


@pytest.mark.parametrize(
    ("header", "payload"),
    [
        (
            b'{"alg":"RS256","alg":"RS256","typ":"JWT"}',
            b'{"iss":"blog-api"}',
        ),
        (
            b'{"alg":"RS256","typ":"JWT"}',
            b'{"iss":"blog-api","iss":"blog-api"}',
        ),
        (
            b'{"alg":"none","typ":"JWT"}',
            b'{"iss":"blog-api"}',
        ),
    ],
)
def test_structural_extensions_fail_closed(
    verifier: JwtVerifier,
    private_key: rsa.RSAPrivateKey,
    header: bytes,
    payload: bytes,
) -> None:
    # Given: 重复头、重复声明或 none 算法结构。
    token = compact(private_key, header, payload)
    # When/Then: 在不回显结构内容的情况下拒绝。
    with pytest.raises(AuthenticationError) as captured:
        _ = verifier.verify(token)
    assert str(captured.value) == "身份令牌无效"
    assert repr(token) == "EncodedJwt(***)"


def test_clock_skew_boundaries_are_inclusive(
    verifier: JwtVerifier,
    private_key: rsa.RSAPrivateKey,
) -> None:
    # Given: exp、nbf 与 iat 恰好位于配置偏差边界。
    expiry_payload = {**blog_claims(), "exp": NOW - 29, "nbf": NOW - 30, "iat": NOW - 30}
    future_payload = {**blog_claims(), "nbf": NOW + 30, "iat": NOW + 30}
    tokens = (
        EncodedJwt(jwt.encode(expiry_payload, private_key, algorithm="RS256")),
        EncodedJwt(jwt.encode(future_payload, private_key, algorithm="RS256")),
    )
    # When/Then: 边界内令牌仍解析为配置服务主体。
    for token in tokens:
        principal = verifier.verify(token)
        assert type(principal) is BlogServicePrincipal
        assert principal.subject == "blog-api-service"
