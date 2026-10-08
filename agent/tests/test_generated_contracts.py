"""Generated protobuf wire and presence behavior."""

from scyg_agent.generated.proto.scyg.agent.v1 import agent_control_service_pb2 as agent_pb
from scyg_agent.generated.proto.scyg.blog.v1 import blog_content_service_pb2 as blog_pb


def test_agent_resume_preserves_absent_and_explicit_null_payload() -> None:
    request = agent_pb.ResumeRunRequest(
        user_id="user-1",
        run_id="run_12345678",
        idempotency_key="46aaeb1e-b23a-439e-a5b1-f9346e2f4629",
        interaction_id="interaction-1",
        decision="approve",
    )
    absent = agent_pb.ResumeRunRequest.FromString(request.SerializeToString())
    assert not absent.HasField("payload_json")
    request.payload_json = b"null"
    explicit = agent_pb.ResumeRunRequest.FromString(request.SerializeToString())
    assert explicit.HasField("payload_json")
    assert explicit.payload_json == b"null"


def test_blog_update_preserves_omitted_and_empty_replacements() -> None:
    request = blog_pb.UpdateArticleRequest(
        user_id="user-1",
        operation_id="46aaeb1e-b23a-439e-a5b1-f9346e2f4629",
        article_id=42,
        expected_version=3,
    )
    absent = blog_pb.UpdateArticleRequest.FromString(request.SerializeToString())
    assert not absent.HasField("title")
    assert not absent.HasField("tag_ids")
    request.title = ""
    request.tag_ids.CopyFrom(blog_pb.TagIds())
    explicit = blog_pb.UpdateArticleRequest.FromString(request.SerializeToString())
    assert explicit.HasField("title")
    assert explicit.title == ""
    assert explicit.HasField("tag_ids")
    assert list(explicit.tag_ids.values) == []


def test_sse_frame_round_trip_preserves_bytes() -> None:
    frame = b'id: 123-0\nevent: completed\ndata: {"title":"caf\xc3\xa9"}\n\n'
    message = agent_pb.RunEventFrame(frame=frame)
    assert agent_pb.RunEventFrame.FromString(message.SerializeToString()).frame == frame
