"""
Setup script with post-install hook for sitecustomize.py installation.
"""

from distutils.core import setup as distutils_setup
from setuptools import setup
from setuptools.command.install import install

try:
    from install_hooks import SitecustomizeInstaller
except Exception:
    class SitecustomizeInstaller:  # type: ignore[no-redef]
        """Fallback when install_hooks is unavailable in isolated build envs."""

        def install(self):
            return None


class PostInstallCommand(install):
    """Custom install command to run sitecustomize installer after setup"""

    def run(self):
        install.run(self)
        print("[govagn] Running post-install hook...")
        try:
            SitecustomizeInstaller().install()
        except Exception as e:
            print(f"[govagn] Warning: post-install hook failed: {e}")


setup(
    cmdclass={
        "install": PostInstallCommand,
    },
)
