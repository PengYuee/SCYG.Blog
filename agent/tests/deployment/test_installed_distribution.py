"""Consumer probes against the noneditable installed distribution."""

import subprocess
import sys
from pathlib import Path


def test_generated_bindings_import_without_repository_pythonpath(tmp_path: Path) -> None:
    probe = """
import json
from importlib.metadata import distribution
from pathlib import Path
import scyg_agent.generated.proto.scyg.agent.v1.agent_control_service_pb2 as agent_messages
from grpc_health.v1 import health_pb2
from grpc_status import rpc_status
from google.rpc import status_pb2
from scyg_agent.generated.proto.scyg.agent.v1.agent_control_service_pb2 import RunEventFrame
from scyg_agent.generated.proto.scyg.blog.v1.blog_content_service_pb2 import (
    UpdateArticleRequest, TagIds,
)
from scyg_agent.generated.proto.scyg.agent.v1.agent_control_service_pb2_grpc import (
    AgentControlServiceStub,
)
from scyg_agent.generated.proto.scyg.blog.v1.blog_content_service_pb2_grpc import (
    BlogContentServiceStub,
)
installed = distribution('scyg-agent')
direct_url = installed.read_text('direct_url.json')
assert direct_url is None or not json.loads(direct_url).get('dir_info', {}).get('editable', False)
installed_package = installed.locate_file('scyg_agent').resolve()
assert Path(agent_messages.__file__).resolve().is_relative_to(installed_package)
health = health_pb2.HealthCheckRequest(service='scyg.agent.v1.AgentControlService')
assert health_pb2.HealthCheckRequest.FromString(health.SerializeToString()) == health
status = rpc_status.to_status(status_pb2.Status(code=3, message='Invalid request'))
details = dict(status.trailing_metadata)['grpc-status-details-bin']
assert status_pb2.Status.FromString(details).message == 'Invalid request'
frame = b'id: cursor\\nevent: run_succeeded\\ndata: {}\\n\\n'
assert RunEventFrame.FromString(RunEventFrame(frame=frame).SerializeToString()).frame == frame
update = UpdateArticleRequest(article_id=42, expected_version=3)
assert not update.HasField('tag_ids')
update.tag_ids.CopyFrom(TagIds())
assert UpdateArticleRequest.FromString(update.SerializeToString()).HasField('tag_ids')
"""
    result = subprocess.run(  # noqa: S603 - fixed interpreter and repository-owned probe.
        [sys.executable, "-I", "-c", probe],
        cwd=tmp_path,
        capture_output=True,
        text=True,
        check=False,
        timeout=30,
    )
    assert result.returncode == 0, result.stderr
