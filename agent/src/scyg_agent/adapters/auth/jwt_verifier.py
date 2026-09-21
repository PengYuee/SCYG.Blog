"""固定 RS256 与封闭声明策略的 JWT 验证器。."""

from __future__ import annotations

import base64
import json
from dataclasses import dataclass, field
from datetime import UTC, datetime
from typing import TYPE_CHECKING, Final, Protocol

import jwt
from jwt.exceptions import InvalidTokenError
from pydantic import JsonValue, TypeAdapter, ValidationError

from scyg_agent.domain.runs.errors import InvalidIdentifierError

from .claims import BlogServiceClaims, PrincipalKindClaims, WebRunClaims
from .errors import AuthenticationError, AuthenticationErrorCode
from .principals import BlogServicePrincipal, Principal, WebRunPrincipal

if TYPE_CHECKING:
    from cryptography.hazmat.primitives.asymmetric.rsa import RSAPublicKey

    from .credentials import EncodedJwt

ALGORITHM: Final = "RS256"
JWT_TYPE: Final = "JWT"
MAX_CLOCK_SKEW_SECONDS: Final = 300
BLOG_SCOPES: Final = frozenset(("agent:runs:create", "agent:runs:read"))
WEB_SCOPES: Final = frozenset(("run:read", "run:command", "run:cancel"))
JSON_ADAPTER: Final[TypeAdapter[JsonValue]] = TypeAdapter(JsonValue)
DECODED_PAYLOAD_ADAPTER: Final[TypeAdapter[dict[str, JsonValue]]] = TypeAdapter(
    dict[str, JsonValue]
)
CLAIMS_ADAPTER: Final[TypeAdapter[BlogServiceClaims | WebRunClaims]] = TypeAdapter(
    BlogServiceClaims | WebRunClaims
)


class Clock(Protocol):
    """提供可注入的 UTC 时间。."""

    def now(self) -> datetime:
        """返回当前 UTC 时刻。."""
        ...


@dataclass(frozen=True, slots=True)
class SystemClock:
    """读取系统 UTC 时间。."""

    def now(self) -> datetime:
        """返回系统当前 UTC 时刻。."""
        return datetime.now(tz=UTC)


@dataclass(frozen=True, slots=True)
class AuthPolicy:
    """保存验证器唯一不可变信任根与声明策略。."""

    public_key: RSAPublicKey = field(repr=False)
    issuer: str
    audience: str
    blog_service_subject: str
    clock_skew_seconds: int = 30

    def __post_init__(self) -> None:
        """拒绝空配置、布尔偏差及无界时钟偏差。."""
        if (
            type(self.clock_skew_seconds) is not int
            or not 0 <= self.clock_skew_seconds <= MAX_CLOCK_SKEW_SECONDS
            or not self.issuer
            or not self.audience
            or not self.blog_service_subject
        ):
            raise AuthenticationError(AuthenticationErrorCode.REJECTED)


class DuplicateJsonMemberError(ValueError):
    """表示 JWT JSON 对象含重复成员。."""


def _unique_object(pairs: list[tuple[str, JsonValue]]) -> dict[str, JsonValue]:
    """构建 JSON 对象并拒绝重复键。."""
    result: dict[str, JsonValue] = {}
    for name, value in pairs:
        if name in result:
            raise DuplicateJsonMemberError
        result[name] = value
    return result


def _decode_segment(segment: str) -> bytes:
    """严格解码一个 JWT base64url 段。."""
    padding = "=" * (-len(segment) % 4)
    return base64.b64decode(segment + padding, altchars=b"-_", validate=True)


def _load_json(segment: str) -> JsonValue:
    """解析 JSON 并将第三方动态返回值收窄为封闭 JSON 类型."""
    return JSON_ADAPTER.validate_python(
        json.loads(_decode_segment(segment), object_pairs_hook=_unique_object)
    )


def _token_segments(token: EncodedJwt) -> tuple[str, str]:
    """解析紧凑序列化并要求三个非空段."""
    header, payload, signature = token.value.split(".")
    if not header or not payload or not signature:
        raise AuthenticationError(AuthenticationErrorCode.REJECTED)
    return header, payload


def _validate_header(segment: str) -> None:
    """只允许固定算法与固定 JWT 类型两个头字段."""
    header = _load_json(segment)
    if header != {"alg": ALGORITHM, "typ": JWT_TYPE}:
        raise AuthenticationError(AuthenticationErrorCode.REJECTED)


def _parse_claims(payload: JsonValue) -> BlogServiceClaims | WebRunClaims:
    """先验证封闭判别字段, 再解析精确声明联合."""
    _ = PrincipalKindClaims.model_validate(payload)
    return CLAIMS_ADAPTER.validate_python(payload)


@dataclass(frozen=True, slots=True)
class JwtVerifier:
    """验证签名后将不可信 JWT 转换为封闭主体。."""

    policy: AuthPolicy
    clock: Clock = field(default_factory=SystemClock)

    def verify(self, token: EncodedJwt) -> Principal:
        """验证固定算法、配置身份、时间、作用域及品牌标识。."""
        try:
            header_segment, payload_segment = _token_segments(token)
            _validate_header(header_segment)
            _ = DECODED_PAYLOAD_ADAPTER.validate_python(
                jwt.decode(
                    token.value,
                    self.policy.public_key,
                    algorithms=[ALGORITHM],
                    issuer=self.policy.issuer,
                    audience=self.policy.audience,
                    options={"verify_exp": False, "verify_nbf": False, "verify_iat": False},
                )
            )
            payload = _load_json(payload_segment)
            parsed = _parse_claims(payload)
            self._validate_time(parsed)
            return self._principal(parsed)
        except AuthenticationError:
            raise
        except (
            InvalidTokenError,
            ValidationError,
            InvalidIdentifierError,
            ValueError,
        ):
            raise AuthenticationError(AuthenticationErrorCode.REJECTED) from None

    def require_exact_principal(self, principal: Principal) -> None:
        """防止主体子类冒充封闭授权变体。."""
        if type(principal) not in {BlogServicePrincipal, WebRunPrincipal}:
            raise AuthenticationError(AuthenticationErrorCode.INVALID_PRINCIPAL)

    def _validate_time(self, claims: BlogServiceClaims | WebRunClaims) -> None:
        """使用注入时钟和有界偏差校验全部时间关系。."""
        now = int(self.clock.now().timestamp())
        skew = self.policy.clock_skew_seconds
        if (
            claims.exp <= now - skew
            or claims.nbf > now + skew
            or claims.iat > now + skew
            or claims.iat > claims.exp
            or claims.nbf > claims.exp
        ):
            raise AuthenticationError(AuthenticationErrorCode.REJECTED)

    def _principal(self, claims: BlogServiceClaims | WebRunClaims) -> Principal:
        """按精确声明变体与最小权限集合创建主体。."""
        if type(claims) not in {BlogServiceClaims, WebRunClaims}:
            raise AuthenticationError(AuthenticationErrorCode.REJECTED)
        if isinstance(claims, BlogServiceClaims):
            if (
                claims.sub != self.policy.blog_service_subject
                or len(claims.scope) != len(BLOG_SCOPES)
                or frozenset(claims.scope) != BLOG_SCOPES
            ):
                raise AuthenticationError(AuthenticationErrorCode.REJECTED)
            return BlogServicePrincipal(claims.sub)
        if (
            claims.sub != claims.user_id
            or len(claims.scope) != len(WEB_SCOPES)
            or frozenset(claims.scope) != WEB_SCOPES
        ):
            raise AuthenticationError(AuthenticationErrorCode.REJECTED)
        return WebRunPrincipal.from_strings(claims.user_id, claims.run_id)
