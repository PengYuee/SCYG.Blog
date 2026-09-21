"""严格 RS256 JWT 验证测试。"""

from collections.abc import Callable
from dataclasses import dataclass
from datetime import UTC, datetime
from typing import Final

import jwt
import pytest
from cryptography.hazmat.primitives.asymmetric import rsa

from scyg_agent.adapters.auth import (
    AuthenticationError,
    AuthPolicy,
    BlogServicePrincipal,
    EncodedJwt,
    JwtVerifier,
    WebRunPrincipal,
)

NOW: Final = 2_000_000_000
ISSUER: Final = "https://blog.example.test"
AUDIENCE: Final = "agent-service"
SERVICE_SUBJECT: Final = "blog-api"
RUN_ID: Final = "run_12345678"
USER_ID: Final = "user-123"


@dataclass(frozen=True, slots=True)
class FixedClock:
    """提供确定性 UTC 当前时间。"""

    timestamp: int = NOW

    def now(self) -> datetime:
        """返回固定 UTC 时刻。"""
        return datetime.fromtimestamp(self.timestamp, tz=UTC)


@pytest.fixture(scope="module")
def private_key() -> rsa.RSAPrivateKey:
    """在内存中创建测试专用签名密钥, 不持久化材料."""
    return rsa.generate_private_key(public_exponent=65537, key_size=2048)


@pytest.fixture
def verifier(private_key: rsa.RSAPrivateKey) -> JwtVerifier:
    """构造固定策略、固定时钟和固定测试公钥的验证器。"""
    policy = AuthPolicy(
        public_key=private_key.public_key(),
        issuer=ISSUER,
        audience=AUDIENCE,
        blog_service_subject=SERVICE_SUBJECT,
        clock_skew_seconds=30,
    )
    return JwtVerifier(policy, FixedClock())


def claims(kind: str = "web_run") -> dict[str, str | int | list[str]]:
    """返回一份可按测试修改的合法声明。"""
    common: dict[str, str | int | list[str]] = {
        "iss": ISSUER,
        "aud": AUDIENCE,
        "sub": USER_ID,
        "exp": NOW + 300,
        "nbf": NOW - 30,
        "iat": NOW - 30,
        "principal_kind": kind,
    }
    if kind == "blog_service":
        common["sub"] = SERVICE_SUBJECT
        common["scope"] = ["agent:runs:create", "agent:runs:read"]
    else:
        common["user_id"] = USER_ID
        common["run_id"] = RUN_ID
        common["scope"] = ["run:read", "run:command", "run:cancel"]
    return common


def encode(
    private_key: rsa.RSAPrivateKey,
    payload: dict[str, str | int | list[str]],
    *,
    algorithm: str = "RS256",
    headers: dict[str, str] | None = None,
) -> EncodedJwt:
    """签发仅存在于内存的测试令牌。"""
    return EncodedJwt(jwt.encode(payload, private_key, algorithm=algorithm, headers=headers))


def test_verify_returns_exact_frozen_principal_variants(
    verifier: JwtVerifier,
    private_key: rsa.RSAPrivateKey,
) -> None:
    # Given: 两种精确合法的最小权限声明。
    blog_token = encode(private_key, claims("blog_service"))
    web_token = encode(private_key, claims())
    # When: 分别验证服务令牌和 Run 令牌。
    blog = verifier.verify(blog_token)
    web = verifier.verify(web_token)
    # Then: 返回封闭、精确、不可变的主体变体。
    assert type(blog) is BlogServicePrincipal
    assert blog == BlogServicePrincipal(SERVICE_SUBJECT)
    assert type(web) is WebRunPrincipal
    assert web == WebRunPrincipal.from_strings(USER_ID, RUN_ID)
    assert hash(web)


Mutation = Callable[[dict[str, str | int | list[str]]], None]


def remove(name: str) -> Mutation:
    """创建删除一个声明的测试变换。"""

    def mutate(payload: dict[str, str | int | list[str]]) -> None:
        _ = payload.pop(name)

    return mutate


def replace(name: str, value: str | int | list[str]) -> Mutation:
    """创建替换一个声明的测试变换。"""

    def mutate(payload: dict[str, str | int | list[str]]) -> None:
        payload[name] = value

    return mutate


@pytest.mark.parametrize(
    "mutation",
    [
        remove("iss"),
        remove("aud"),
        remove("sub"),
        remove("exp"),
        remove("nbf"),
        remove("iat"),
        remove("scope"),
        remove("user_id"),
        remove("run_id"),
        replace("iss", "wrong"),
        replace("aud", "wrong"),
        replace("sub", "wrong"),
        replace("exp", NOW - 31),
        replace("nbf", NOW + 31),
        replace("iat", NOW + 31),
        replace("principal_kind", "unknown"),
        replace("run_id", "bad"),
        replace("user_id", " bad"),
        replace("scope", ["run:read", "run:command"]),
        replace("scope", ["run:read", "run:command", "run:cancel", "run:read"]),
    ],
)
def test_verify_rejects_invalid_web_claim_matrix(
    verifier: JwtVerifier,
    private_key: rsa.RSAPrivateKey,
    mutation: Mutation,
) -> None:
    # Given: 一个声明被删除、改错、越界或重复。
    payload = claims()
    mutation(payload)
    token = encode(private_key, payload)
    # When/Then: 只暴露统一中文错误, 不泄露声明或令牌。
    with pytest.raises(AuthenticationError, match="^身份令牌无效$"):
        _ = verifier.verify(token)


@pytest.mark.parametrize(
    ("payload", "headers"),
    [
        ({**claims(), "extra": "value"}, None),
        (claims(), {"kid": "attacker"}),
        (claims(), {"jku": "https://attacker.invalid/key"}),
    ],
)
def test_verify_rejects_extra_claims_and_untrusted_headers(
    verifier: JwtVerifier,
    private_key: rsa.RSAPrivateKey,
    payload: dict[str, str | int | list[str]],
    headers: dict[str, str] | None,
) -> None:
    # Given: 额外声明或可选择外部密钥的头字段。
    token = encode(private_key, payload, headers=headers)
    # When/Then: 验证器关闭该扩展面。
    with pytest.raises(AuthenticationError, match="^身份令牌无效$"):
        _ = verifier.verify(token)


def test_verify_rejects_wrong_algorithm_and_signature(
    verifier: JwtVerifier,
) -> None:
    # Given: HS256 令牌与另一 RSA 密钥签发的令牌。
    hs_token = EncodedJwt(
        jwt.encode(claims(), "test-hmac-key-with-at-least-32-bytes", algorithm="HS256")
    )
    other_key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    other_token = encode(other_key, claims())
    # When/Then: 算法与签名均被固定策略拒绝。
    for token in (hs_token, other_token):
        with pytest.raises(AuthenticationError, match="^身份令牌无效$"):
            _ = verifier.verify(token)


def test_verify_rejects_wrong_scalar_types_and_principal_subclasses(
    verifier: JwtVerifier,
    private_key: rsa.RSAPrivateKey,
) -> None:
    # Given: bool 时间、标量 scope, 以及试图冒充主体的子类。
    malformed = ({**claims(), "exp": True}, {**claims(), "scope": "run:read"})

    @dataclass(frozen=True, slots=True)
    class WebImpostor(WebRunPrincipal):
        """模拟继承合法字段的主体冒充值。"""

    # When/Then: 外部标量和内部子类均不能进入可信主体闭集。
    for payload in malformed:
        with pytest.raises(AuthenticationError, match="^身份令牌无效$"):
            _ = verifier.verify(encode(private_key, payload))
    with pytest.raises(AuthenticationError, match="^身份主体无效$"):
        verifier.require_exact_principal(WebImpostor.from_strings(USER_ID, RUN_ID))
