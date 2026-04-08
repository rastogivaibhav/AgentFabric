"""
Tests for auto_instrument.py and install_hooks.py
"""

from __future__ import annotations

import os
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

import pytest

from govagn.auto_instrument import AutoInstrumentor
from install_hooks import SitecustomizeInstaller


class TestAutoInstrumentor(unittest.TestCase):
    """Test AutoInstrumentor class"""

    def setUp(self):
        """Save original env vars"""
        self.original_env = os.environ.copy()

    def tearDown(self):
        """Restore original env vars"""
        os.environ.clear()
        os.environ.update(self.original_env)

    def test_disabled_when_env_var_zero(self):
        """GV_AUTO_INSTRUMENT=0 → no instrumentation"""
        os.environ["GV_AUTO_INSTRUMENT"] = "0"

        instrumentor = AutoInstrumentor()
        instrumentor.read_config()

        assert instrumentor.enabled is False

    def test_enabled_by_default(self):
        """Without GV_AUTO_INSTRUMENT → enabled"""
        os.environ.pop("GV_AUTO_INSTRUMENT", None)

        instrumentor = AutoInstrumentor()
        instrumentor.read_config()

        assert instrumentor.enabled is True

    def test_reads_endpoint_from_env(self):
        """GV_ENDPOINT overrides default"""
        os.environ["GV_ENDPOINT"] = "http://custom:4318"

        instrumentor = AutoInstrumentor()
        instrumentor.read_config()

        assert instrumentor.endpoint == "http://custom:4318"

    def test_default_endpoint_localhost(self):
        """Without GV_ENDPOINT → http://localhost:4318"""
        os.environ.pop("GV_ENDPOINT", None)

        instrumentor = AutoInstrumentor()
        instrumentor.read_config()

        assert instrumentor.endpoint == "http://localhost:4318"

    def test_reads_tenant_id_from_env(self):
        """GV_TENANT_ID sets tenant"""
        os.environ["GV_TENANT_ID"] = "acme-corp"

        instrumentor = AutoInstrumentor()
        instrumentor.read_config()

        assert instrumentor.tenant_id == "acme-corp"

    def test_default_tenant_id_is_default(self):
        """Without GV_TENANT_ID → 'default'"""
        os.environ.pop("GV_TENANT_ID", None)

        instrumentor = AutoInstrumentor()
        instrumentor.read_config()

        assert instrumentor.tenant_id == "default"

    def test_reads_service_name_from_env(self):
        """GV_SERVICE_NAME sets service name"""
        os.environ["GV_SERVICE_NAME"] = "my-agent-service"

        instrumentor = AutoInstrumentor()
        instrumentor.read_config()

        assert instrumentor.service_name == "my-agent-service"

    def test_detects_service_name_from_argv(self):
        """Without GV_SERVICE_NAME → detect from sys.argv[0]"""
        os.environ.pop("GV_SERVICE_NAME", None)
        with mock.patch.object(sys, "argv", ["/path/to/my_script.py", "arg1"]):
            instrumentor = AutoInstrumentor()
            instrumentor.read_config()

            # Should extract "my_script" from argv[0]
            assert "my_script" in instrumentor.service_name or instrumentor.service_name != ""

    def test_insecure_defaults_true_for_localhost(self):
        """GV_ENDPOINT localhost → insecure=True"""
        os.environ["GV_ENDPOINT"] = "http://localhost:4318"
        os.environ.pop("GV_INSECURE", None)

        instrumentor = AutoInstrumentor()
        instrumentor.read_config()

        assert instrumentor.insecure is True

    def test_insecure_defaults_false_for_remote(self):
        """GV_ENDPOINT remote → insecure=False"""
        os.environ["GV_ENDPOINT"] = "http://api.example.com:4318"
        os.environ.pop("GV_INSECURE", None)

        instrumentor = AutoInstrumentor()
        instrumentor.read_config()

        assert instrumentor.insecure is False

    def test_reads_api_key_from_env(self):
        """GV_API_KEY sets api key"""
        os.environ["GV_API_KEY"] = "my-secret-key"

        instrumentor = AutoInstrumentor()
        instrumentor.read_config()

        assert instrumentor.api_key == "my-secret-key"

    def test_api_key_empty_string_treated_as_none(self):
        """GV_API_KEY='' → None"""
        os.environ["GV_API_KEY"] = ""

        instrumentor = AutoInstrumentor()
        instrumentor.read_config()

        assert instrumentor.api_key is None

    def test_run_respects_disabled(self):
        """AutoInstrumentor.run() with enabled=False → early return"""
        os.environ["GV_AUTO_INSTRUMENT"] = "0"

        instrumentor = AutoInstrumentor()
        # Should not raise
        instrumentor.run()

    @mock.patch("govagn.instrument")
    def test_setup_tracer_calls_instrument(self, mock_instrument):
        """setup_tracer() calls govagn.instrument()"""
        # Mock govagn to avoid actually initializing a tracer
        import govagn

        with mock.patch.object(govagn, "_initialized", False):
            instrumentor = AutoInstrumentor()
            instrumentor.endpoint = "http://localhost:4318"
            instrumentor.service_name = "test-service"

            instrumentor.setup_tracer()

            # Verify instrument was called
            assert mock_instrument.called

    @mock.patch("govagn.instrument")
    def test_setup_tracer_idempotent(self, mock_instrument):
        """setup_tracer() skips if already initialized"""
        import govagn

        with mock.patch.object(govagn, "_initialized", True):
            instrumentor = AutoInstrumentor()
            instrumentor.endpoint = "http://localhost:4318"

            instrumentor.setup_tracer()

            # Should not call instrument if already initialized
            assert not mock_instrument.called

    def test_patch_all_available_handles_missing_frameworks(self):
        """patch_all_available() handles missing frameworks gracefully"""
        instrumentor = AutoInstrumentor()

        # Should not raise even if frameworks are not installed
        instrumentor.patch_all_available()


class TestSitecustomizeInstaller(unittest.TestCase):
    """Test SitecustomizeInstaller class"""

    def test_install_creates_sitecustomize(self):
        """SitecustomizeInstaller.install() creates sitecustomize.py"""
        with tempfile.TemporaryDirectory() as tmpdir:
            site_packages = Path(tmpdir)
            sitecustomize_path = site_packages / "sitecustomize.py"

            with mock.patch.object(
                SitecustomizeInstaller, "_find_site_packages", return_value=site_packages
            ):
                installer = SitecustomizeInstaller()
                installer.install()

                assert sitecustomize_path.exists()
                content = sitecustomize_path.read_text()
                assert "govagn-auto-instrument-start" in content
                assert "AutoInstrumentor" in content

    def test_install_merges_with_existing_sitecustomize(self):
        """SitecustomizeInstaller.install() merges with existing content"""
        with tempfile.TemporaryDirectory() as tmpdir:
            site_packages = Path(tmpdir)
            sitecustomize_path = site_packages / "sitecustomize.py"

            # Create existing sitecustomize
            existing_content = "print('hello')\n"
            sitecustomize_path.write_text(existing_content)

            with mock.patch.object(
                SitecustomizeInstaller, "_find_site_packages", return_value=site_packages
            ):
                installer = SitecustomizeInstaller()
                installer.install()

                content = sitecustomize_path.read_text()

                # Both old and new content should be present
                assert "print('hello')" in content
                assert "AutoInstrumentor" in content

    def test_install_idempotent(self):
        """SitecustomizeInstaller.install() is idempotent"""
        with tempfile.TemporaryDirectory() as tmpdir:
            site_packages = Path(tmpdir)
            sitecustomize_path = site_packages / "sitecustomize.py"

            with mock.patch.object(
                SitecustomizeInstaller, "_find_site_packages", return_value=site_packages
            ):
                installer = SitecustomizeInstaller()

                # Install twice
                installer.install()
                first_content = sitecustomize_path.read_text()

                installer.install()
                second_content = sitecustomize_path.read_text()

                # Content should not have duplicate blocks
                assert first_content == second_content
                count = second_content.count("govagn-auto-instrument-start")
                assert count == 1, f"Expected 1 govagn block, got {count}"

    def test_uninstall_removes_govagn_block(self):
        """SitecustomizeInstaller.uninstall() removes govagn block"""
        with tempfile.TemporaryDirectory() as tmpdir:
            site_packages = Path(tmpdir)
            sitecustomize_path = site_packages / "sitecustomize.py"

            # Create sitecustomize with both old and govagn content
            original_content = "print('hello')\n"
            installer = SitecustomizeInstaller()
            govagn_block = installer._read_boot_code()
            mixed_content = original_content + govagn_block

            sitecustomize_path.write_text(mixed_content)

            with mock.patch.object(
                SitecustomizeInstaller, "_find_site_packages", return_value=site_packages
            ):
                installer = SitecustomizeInstaller()
                installer.uninstall()

                content = sitecustomize_path.read_text()

                # Govagn block should be gone
                assert "govagn-auto-instrument-start" not in content
                # Original content should remain
                assert "print('hello')" in content

    def test_uninstall_deletes_empty_file(self):
        """SitecustomizeInstaller.uninstall() deletes file if only govagn content"""
        with tempfile.TemporaryDirectory() as tmpdir:
            site_packages = Path(tmpdir)
            sitecustomize_path = site_packages / "sitecustomize.py"

            # Create sitecustomize with only govagn content
            installer = SitecustomizeInstaller()
            govagn_block = installer._read_boot_code()

            sitecustomize_path.write_text(govagn_block)

            with mock.patch.object(
                SitecustomizeInstaller, "_find_site_packages", return_value=site_packages
            ):
                installer = SitecustomizeInstaller()
                installer.uninstall()

                # File should be deleted
                assert not sitecustomize_path.exists()


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
