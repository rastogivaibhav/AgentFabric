"""
Setup script with post-install hook for sitecustomize.py installation.
"""

from distutils.core import setup as distutils_setup
from setuptools import setup
from setuptools.command.install import install

from install_hooks import SitecustomizeInstaller


class PostInstallCommand(install):
    """Custom install command to run sitecustomize installer after setup"""

    def run(self):
        install.run(self)
        print("[agentfabric] Running post-install hook...")
        try:
            SitecustomizeInstaller().install()
        except Exception as e:
            print(f"[agentfabric] Warning: post-install hook failed: {e}")


setup(
    cmdclass={
        "install": PostInstallCommand,
    },
)
