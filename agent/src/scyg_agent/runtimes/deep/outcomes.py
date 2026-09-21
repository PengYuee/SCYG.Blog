"""将 T16 封闭结果映射为 T12 工具终态."""

from dataclasses import replace

from scyg_agent.adapters.blog_grpc import (
    ArticleSucceeded,
    BlogFailure,
    BlogResult,
    SearchSucceeded,
)
from scyg_agent.domain.ports.idempotency import ResultMetadata, ResultReference
from scyg_agent.domain.ports.tool_store import ToolOperation, ToolOutcomeStatus

from .models import ProposalEnvelope


class BlogOutcomeFactory:
    """使用 proposal 的持久身份生成无提供方正文终态."""

    def from_blog_result(
        self,
        proposal: ProposalEnvelope,
        result: BlogResult,
    ) -> ToolOperation:
        """穷尽映射 T16 成功和失败闭集."""
        base = proposal.fallback_outcome
        match result:  # noqa: RUF100  # noqa: MATCH_OK - BlogResult 全部分支已映射。
            case ArticleSucceeded(article=article):
                return replace(
                    base,
                    status=ToolOutcomeStatus.SUCCEEDED,
                    result_reference=ResultReference(
                        f"article:{article.article_id}:version:{article.version}"
                    ),
                    metadata=ResultMetadata("article_version", str(article.version)),
                    error_code=None,
                )
            case SearchSucceeded(articles=articles):
                return replace(
                    base,
                    status=ToolOutcomeStatus.SUCCEEDED,
                    result_reference=ResultReference(f"search:{base.tool_call_id}"),
                    metadata=ResultMetadata("result_count", str(len(articles))),
                    error_code=None,
                )
            case BlogFailure(kind=kind, retryable=retryable):
                return replace(
                    base,
                    status=ToolOutcomeStatus.FAILED,
                    result_reference=None,
                    metadata=ResultMetadata("retryable", str(retryable).lower()),
                    error_code=kind.value,
                )
