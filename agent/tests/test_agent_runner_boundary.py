"""AgentRunner structured output boundary tests."""

import pytest
from pydantic import ValidationError

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
from scyg_agent.agents.contracts import ArticleEvidence, WritingInput


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


@pytest.mark.parametrize("article_id", [1, 2**63 - 1])
def test_resource_ids_remain_numbers_in_capability_json(article_id: int) -> None:
    """All resource references preserve Blog int64 IDs through JSON serialization."""
    writing = WritingInput.model_validate({"topic": "Topic", "reference_article_ids": [article_id]})
    evidence = ArticleEvidence.model_validate(
        {
            "article_id": article_id,
            "title": "Title",
            "visibility": "published",
            "evidence": "Source",
        },
    )
    draft = ArticleDraft.model_validate(
        {
            "title": "Title",
            "outline": "Outline",
            "markdown": "Content",
            "source_article_ids": [article_id],
        },
    )
    chat = ChatResponse.model_validate({"response": "Answer", "source_article_ids": [article_id]})
    assert writing.reference_article_ids == (article_id,)
    assert writing.model_dump(mode="json")["reference_article_ids"] == [article_id]
    assert evidence.model_dump(mode="json")["article_id"] == article_id
    assert output_payload(draft)["source_article_ids"] == [article_id]
    assert output_payload(chat)["source_article_ids"] == [article_id]


@pytest.mark.parametrize("article_id", [True, False, "1", "article-1", 1.0, 0, -1, 2**63])
def test_resource_ids_reject_non_int64_values(article_id: object) -> None:
    """References never coerce strings, booleans or out-of-range numeric values."""
    cases = (
        (WritingInput, {"topic": "Topic", "reference_article_ids": [article_id]}),
        (
            ArticleEvidence,
            {
                "article_id": article_id,
                "title": "Title",
                "visibility": "published",
                "evidence": "Source",
            },
        ),
        (
            ArticleDraft,
            {
                "title": "Title",
                "outline": "Outline",
                "markdown": "Content",
                "source_article_ids": [article_id],
            },
        ),
        (ChatResponse, {"response": "Answer", "source_article_ids": [article_id]}),
    )
    for schema, payload in cases:
        with pytest.raises(ValidationError):
            _ = schema.model_validate(payload)


def test_writing_reference_limit_survives_numeric_cutover() -> None:
    """The existing reference expansion limit still applies to numeric IDs."""
    with pytest.raises(ValidationError):
        _ = WritingInput.model_validate(
            {
                "topic": "Topic",
                "reference_article_ids": list(range(1, 22)),
            }
        )


@pytest.mark.parametrize("visibility", ["draft", "published", "archived"])
def test_search_evidence_accepts_authorized_management_states(visibility: str) -> None:
    evidence = ArticleEvidence.model_validate(
        {
            "article_id": 17,
            "title": "Article",
            "visibility": visibility,
            "evidence": "Tool-backed content",
        }
    )
    assert evidence.visibility == visibility
