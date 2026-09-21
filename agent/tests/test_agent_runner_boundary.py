"""AgentRunner structured output boundary tests."""

import pytest

from scyg_agent.agents import (
    AgentSucceeded,
    ArticleDraft,
    Capability,
    ChatResponse,
    InvalidCapabilityOutputError,
    output_digest,
    output_payload,
    validate_capability_output,
    validated_agent_success,
)


def test_runner_accepts_only_capability_matching_structured_output() -> None:
    """A successful Runner outcome carries the exact capability schema."""
    output = ArticleDraft(title="标题", outline="大纲", markdown="# 正文")

    accepted = validated_agent_success(Capability.WRITE, output)

    assert isinstance(accepted, AgentSucceeded)
    assert accepted.output is output
    assert output_payload(output) == {
        "title": "标题",
        "outline": "大纲",
        "markdown": "# 正文",
        "source_article_ids": [],
    }


def test_runner_rejects_cross_capability_output() -> None:
    """A result for one capability cannot be persisted as another capability."""
    output = ChatResponse(response="回答")

    with pytest.raises(InvalidCapabilityOutputError):
        _ = validate_capability_output(Capability.SEARCH, output)


def test_output_digest_is_canonical_and_content_sensitive() -> None:
    """The persisted result digest is stable for equal content and changes on edits."""
    first = ArticleDraft(title="标题", outline="大纲", markdown="# 正文")
    equal = ArticleDraft(title="标题", outline="大纲", markdown="# 正文")
    changed = ArticleDraft(title="标题", outline="大纲", markdown="# 修改")

    assert output_digest(first) == output_digest(equal)
    assert output_digest(first) != output_digest(changed)
