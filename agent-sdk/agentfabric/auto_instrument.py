"""
AutoInstrumentor — sitecustomize.py entry point for auto-instrumentation.

This module is called automatically at Python startup if installed via sitecustomize.py.
It reads environment variables, initializes the tracer, and patches all available frameworks.
Never raises — failures are logged to stderr.
"""

from __future__ import annotations

import importlib
import logging
import os
import sys
from typing import Optional

# Set up stderr logging (sitecustomize runs before user code, so basicConfig is safe)
logging.basicConfig(
    format="[agentfabric] %(levelname)s: %(message)s",
    level=logging.INFO,
    stream=sys.stderr,
)
logger = logging.getLogger("agentfabric.auto_instrument")


class AutoInstrumentor:
    """
    Called once at Python startup from sitecustomize.py.
    Reads environment variables, decides which frameworks to patch, bootstraps tracer.
    """

    ENDPOINT_ENV = "AF_ENDPOINT"  # e.g. http://localhost:4318
    TENANT_ENV = "AF_TENANT_ID"  # e.g. acme-corp
    DISABLED_ENV = "AF_AUTO_INSTRUMENT"  # set to "0" to opt out
    SERVICE_NAME_ENV = "AF_SERVICE_NAME"  # OTel service.name
    API_KEY_ENV = "AF_API_KEY"  # Optional API key for authentication
    INSECURE_ENV = "AF_INSECURE"  # set to "1" for insecure (default true for localhost)
    PROMPT_ID_ENV = "AF_PROMPT_ID"
    PROMPT_VERSION_ENV = "AF_PROMPT_VERSION"
    PROMPT_RELEASE_TAG_ENV = "AF_PROMPT_RELEASE_TAG"
    PROMPT_ENVIRONMENT_ENV = "AF_PROMPT_ENVIRONMENT"

    def __init__(self):
        self.endpoint: Optional[str] = None
        self.tenant_id: Optional[str] = None
        self.service_name: str = "agentfabric-service"
        self.api_key: Optional[str] = None
        self.insecure: bool = True
        self.enabled: bool = True
        self.prompt_id: Optional[str] = None
        self.prompt_version: Optional[str] = None
        self.prompt_release_tag: Optional[str] = None
        self.prompt_environment: Optional[str] = None

    def run(self) -> None:
        """Entry point called by sitecustomize.py"""
        try:
            self.read_config()
            if not self.enabled:
                logger.debug("AF_AUTO_INSTRUMENT=0 — skipping auto-instrumentation")
                return
            self.setup_tracer()
            self.patch_all_available()
            logger.info(
                f"AgentFabric auto-instrumentation active "
                f"(endpoint={self.endpoint}, tenant={self.tenant_id}, "
                f"prompt={self.prompt_id or '-'}:{self.prompt_version or '-'})"
            )
        except Exception as e:
            logger.error(f"AutoInstrumentor.run() failed: {e}", exc_info=False)
            # Never raise — failure here breaks user's Python process

    def read_config(self) -> None:
        """Read AF_ENDPOINT, AF_TENANT_ID, AF_AUTO_INSTRUMENT from env"""
        self.enabled = os.environ.get(self.DISABLED_ENV, "1").strip() != "0"
        self.endpoint = os.environ.get(
            self.ENDPOINT_ENV, "http://localhost:4318"
        ).strip()
        self.tenant_id = os.environ.get(self.TENANT_ENV, "default").strip()
        self.api_key = os.environ.get(self.API_KEY_ENV, "").strip() or None
        self.service_name = os.environ.get(
            self.SERVICE_NAME_ENV, self._detect_service_name()
        ).strip()
        self.prompt_id = os.environ.get(self.PROMPT_ID_ENV, "").strip() or None
        self.prompt_version = os.environ.get(self.PROMPT_VERSION_ENV, "").strip() or None
        self.prompt_release_tag = os.environ.get(self.PROMPT_RELEASE_TAG_ENV, "").strip() or None
        self.prompt_environment = os.environ.get(
            self.PROMPT_ENVIRONMENT_ENV, ""
        ).strip() or None

        # Determine if we should use insecure (gRPC) or HTTPS
        # Default to insecure=True for localhost, insecure=False otherwise
        insecure_env = os.environ.get(self.INSECURE_ENV, "").strip()
        if insecure_env:
            self.insecure = insecure_env != "0"
        else:
            self.insecure = "localhost" in self.endpoint or "127.0.0.1" in self.endpoint

    def _detect_service_name(self) -> str:
        """Auto-detect service name from sys.argv[0] or default"""
        try:
            import os.path

            name = os.path.splitext(os.path.basename(sys.argv[0]))[0]
            return name or "agentfabric-service"
        except Exception:
            return "agentfabric-service"

    def setup_tracer(self) -> None:
        """
        Create OTLPSpanExporter pointing at self.endpoint.
        Create TracerProvider + BatchSpanProcessor.
        Inject into agentfabric._tracer + agentfabric._initialized = True.
        """
        try:
            # Import agentfabric here (not at module level) to avoid circular imports
            import agentfabric

            # Check if already initialized (idempotent)
            if agentfabric._initialized:
                logger.debug("agentfabric already initialized, skipping setup_tracer()")
                return

            # Call the main instrument() function
            # This will set up the tracer and return it
            agentfabric.instrument(
                endpoint=self.endpoint,
                service_name=self.service_name,
                api_key=self.api_key,
                insecure=self.insecure,
                # All frameworks should be patched
                auto_instrument_crewai=True,
                auto_instrument_langgraph=True,
                auto_instrument_openai=True,
                auto_instrument_anthropic=True,
                auto_instrument_google_adk=True,
            )
            logger.debug(
                f"Tracer initialized: endpoint={self.endpoint}, "
                f"service={self.service_name}, insecure={self.insecure}"
            )
        except Exception as e:
            logger.error(f"setup_tracer() failed: {e}", exc_info=False)

    def patch_all_available(self) -> None:
        """
        For each framework, try import — if available, call agentfabric patch fn.
        Never raises — failures are silently logged to stderr.
        Frameworks: openai, anthropic, langgraph, crewai, google.adk
        """
        # These are already patched by instrument(), but we could add custom
        # patches here if needed. For now, just log what was patched.
        frameworks = [
            ("openai", "openai"),
            ("anthropic", "anthropic"),
            ("langgraph", "langgraph"),
            ("crewai", "crewai"),
            ("google.adk", "google_adk"),
        ]

        for module_name, display_name in frameworks:
            try:
                importlib.import_module(module_name)
                logger.debug(f"Framework available: {display_name}")
            except ImportError:
                logger.debug(f"Framework not installed: {display_name}")

    def _try_patch(self, module_name: str, patch_fn_name: str) -> bool:
        """
        importlib.import_module(module_name)
        getattr(agentfabric, patch_fn_name)(module)
        return True on success, False on ImportError/AttributeError
        """
        try:
            module = importlib.import_module(module_name)
            import agentfabric

            patch_fn = getattr(agentfabric, patch_fn_name)
            patch_fn(module)
            logger.debug(f"Patched {module_name} via {patch_fn_name}")
            return True
        except ImportError:
            logger.debug(f"Module {module_name} not installed, skipping patch")
            return False
        except AttributeError:
            logger.error(f"Patch function {patch_fn_name} not found in agentfabric")
            return False
        except Exception as e:
            logger.error(f"Failed to patch {module_name}: {e}")
            return False
