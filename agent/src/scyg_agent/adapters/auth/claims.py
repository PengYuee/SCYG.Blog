"""JWT 声明的严格私有解析模型。."""

from typing import ClassVar, Literal

from pydantic import BaseModel, ConfigDict, StrictInt, StrictStr


class StrictClaims(BaseModel):
    """冻结共有 JWT 声明并拒绝隐式类型转换与额外字段。."""

    model_config: ClassVar[ConfigDict] = ConfigDict(
        frozen=True,
        extra="forbid",
        strict=True,
        hide_input_in_errors=True,
    )

    iss: StrictStr
    aud: StrictStr
    sub: StrictStr
    exp: StrictInt
    nbf: StrictInt
    iat: StrictInt
    scope: list[StrictStr]


class BlogServiceClaims(StrictClaims):
    """Blog 服务令牌的精确声明形状。."""

    principal_kind: Literal["blog_service"]


class WebRunClaims(StrictClaims):
    """Web Run 令牌的精确声明形状。."""

    principal_kind: Literal["web_run"]
    user_id: StrictStr
    run_id: StrictStr


class PrincipalKindClaims(BaseModel):
    """仅解析主体判别字段, 再交给精确变体模型."""

    model_config: ClassVar[ConfigDict] = ConfigDict(
        frozen=True,
        extra="allow",
        strict=True,
        hide_input_in_errors=True,
    )

    principal_kind: Literal["blog_service", "web_run"]
