import importlib.util
from pathlib import Path


def _load_e2e_conftest():
    path = Path(__file__).parent / "e2e" / "conftest.py"
    spec = importlib.util.spec_from_file_location("agentfabric_e2e_conftest", path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


_e2e = _load_e2e_conftest()

sample_otlp_span = _e2e.sample_otlp_span
mock_collector = _e2e.mock_collector
mock_api_gateway = _e2e.mock_api_gateway
tenant_a_headers = _e2e.tenant_a_headers
tenant_b_headers = _e2e.tenant_b_headers
