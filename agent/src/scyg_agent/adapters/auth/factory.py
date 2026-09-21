"""从脱敏配置加载固定 RSA JWT 公钥。."""

from dataclasses import dataclass, field
from pathlib import Path
from typing import override

from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric.rsa import RSAPublicKey
from pydantic import SecretStr

from scyg_agent.config import ApplicationSettings

from .jwt_verifier import AuthPolicy, JwtVerifier


@dataclass(frozen=True, slots=True)
class InvalidJwtPublicKeyError(ValueError):
    """报告不回显路径、PEM 或第三方异常的公钥配置错误。."""

    @override
    def __str__(self) -> str:
        """返回稳定中文诊断。."""
        return "JWT RSA 公钥配置无效"


@dataclass(frozen=True, slots=True)
class JwtPemSource:
    """保存文件或环境秘密包装器中的唯一 PEM 来源。."""

    path: Path | None = field(default=None, repr=False)
    secret: SecretStr | None = field(default=None, repr=False)

    def __post_init__(self) -> None:
        """要求恰好配置一种 PEM 来源。."""
        if (self.path is None) == (self.secret is None):
            raise InvalidJwtPublicKeyError

    def read(self) -> bytes:
        """读取 PEM 且把文件错误收敛为无值诊断。."""
        try:
            if self.path is not None:
                return self.path.read_bytes()
            if self.secret is None:
                raise InvalidJwtPublicKeyError
            return self.secret.get_secret_value().encode()
        except OSError:
            raise InvalidJwtPublicKeyError from None


def load_jwt_rsa_public_key(source: JwtPemSource) -> RSAPublicKey:
    """严格解析一个未加密 RSA PUBLIC KEY 或 RSA PUBLIC KEY PEM。."""
    try:
        key = serialization.load_pem_public_key(source.read())
    except (ValueError, TypeError):
        raise InvalidJwtPublicKeyError from None
    if not isinstance(key, RSAPublicKey):
        raise InvalidJwtPublicKeyError
    return key


def create_jwt_verifier(settings: ApplicationSettings) -> JwtVerifier:
    """从冻结设置创建固定 RS256 验证器。."""
    key = load_jwt_rsa_public_key(JwtPemSource(path=settings.jwt_public_key_path))
    return JwtVerifier(
        AuthPolicy(
            key,
            settings.jwt_issuer.get_secret_value(),
            settings.jwt_audience.get_secret_value(),
            settings.jwt_service_subject.get_secret_value(),
        )
    )
