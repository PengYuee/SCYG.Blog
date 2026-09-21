"""认证凭据边界测试。"""

import pytest

from scyg_agent.adapters.auth import AuthenticationError, EncodedJwt, parse_bearer_values


@pytest.mark.parametrize(
    "values",
    [
        (),
        ("",),
        ("bearer abc",),
        ("Basic abc",),
        ("Bearer",),
        ("Bearer  abc",),
        (" Bearer abc",),
        ("Bearer abc ",),
        ("Bearer abc extra",),
        ("Bearer abc", "Bearer def"),
        (f"Bearer {'a' * 8193}",),
    ],
)
def test_parse_bearer_values_rejects_noncanonical_credentials(
    values: tuple[str, ...],
) -> None:
    # Given: 缺失、重复或非规范的 Authorization/metadata 值。
    # When/Then: 共享边界只返回稳定且不包含输入的中文错误。
    with pytest.raises(AuthenticationError) as captured:
        _ = parse_bearer_values(values)
    assert str(captured.value) in {"缺少身份凭据", "身份凭据格式无效"}
    assert "abc" not in str(captured.value)


def test_parse_bearer_values_returns_typed_token() -> None:
    # Given: 唯一且精确的 Bearer 值。
    # When: 解析共享 HTTP/gRPC 凭据边界。
    token = parse_bearer_values(("Bearer header.payload.signature",))
    # Then: 原始值被封装为不可变类型。
    assert token == EncodedJwt("header.payload.signature")
