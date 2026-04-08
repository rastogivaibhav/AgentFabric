"""
Install hooks for setuptools post-install.

Copies govagn/sitecustomize.py into site-packages/sitecustomize.py,
merging with existing content if present.
"""

from __future__ import annotations

import os
import site
import sys
from pathlib import Path


class SitecustomizeInstaller:
    """
    Called by pyproject.toml [tool.hatch.build.hooks] or setup.py post_install.
    Copies govagn/sitecustomize.py into sys.prefix/lib/.../site-packages/sitecustomize.py.
    Handles merging if a sitecustomize.py already exists (appends govagn block).
    """

    GUARD_START = "# govagn-auto-instrument-start"
    GUARD_END = "# govagn-auto-instrument-end"

    def __init__(self):
        self.site_packages_dir = self._find_site_packages()

    def _find_site_packages(self) -> Path:
        """Find the site-packages directory for this Python."""
        site_packages_dirs = site.getsitepackages()
        if site_packages_dirs:
            return Path(site_packages_dirs[0])

        # Fallback for virtual envs or special installations
        lib_path = Path(sys.prefix) / "lib"
        if lib_path.exists():
            for py_dir in lib_path.iterdir():
                if py_dir.is_dir() and py_dir.name.startswith("python"):
                    sp = py_dir / "site-packages"
                    if sp.exists():
                        return sp

        # Last resort: use sitepackages from site module
        return Path(site.getsitepackages()[0] if site.getsitepackages() else ".")

    def install(self) -> None:
        """Install sitecustomize.py into site-packages, merging if needed."""
        sitecustomize_path = self.site_packages_dir / "sitecustomize.py"
        govagn_boot = self._read_boot_code()

        # Read existing sitecustomize if it exists
        existing_content = ""
        if sitecustomize_path.exists():
            try:
                existing_content = sitecustomize_path.read_text(encoding="utf-8")
            except Exception as e:
                print(f"[govagn] Warning: could not read existing sitecustomize.py: {e}")
                return

        # Check if govagn block is already installed (idempotent)
        if self.GUARD_START in existing_content:
            # Already installed, nothing to do
            return

        # Append govagn boot code
        new_content = existing_content.rstrip() + "\n" + govagn_boot + "\n"

        try:
            sitecustomize_path.write_text(new_content, encoding="utf-8")
            print(f"[govagn] Installed sitecustomize.py to {sitecustomize_path}")
        except Exception as e:
            print(f"[govagn] Error writing sitecustomize.py: {e}")

    def uninstall(self) -> None:
        """Remove govagn block from sitecustomize.py"""
        sitecustomize_path = self.site_packages_dir / "sitecustomize.py"

        if not sitecustomize_path.exists():
            return

        try:
            content = sitecustomize_path.read_text(encoding="utf-8")
        except Exception as e:
            print(f"[govagn] Warning: could not read sitecustomize.py: {e}")
            return

        # Remove govagn block
        lines = content.split("\n")
        in_guard = False
        new_lines = []

        for line in lines:
            if line.strip() == self.GUARD_START:
                in_guard = True
            elif line.strip() == self.GUARD_END:
                in_guard = False
                continue
            elif not in_guard:
                new_lines.append(line)

        new_content = "\n".join(new_lines).strip() + "\n"

        if new_content.strip() == "":
            # File is now empty, delete it
            try:
                sitecustomize_path.unlink()
                print(f"[govagn] Removed empty sitecustomize.py")
            except Exception as e:
                print(f"[govagn] Warning: could not delete sitecustomize.py: {e}")
        else:
            try:
                sitecustomize_path.write_text(new_content, encoding="utf-8")
                print(f"[govagn] Removed govagn block from sitecustomize.py")
            except Exception as e:
                print(f"[govagn] Error writing sitecustomize.py: {e}")

    def _read_boot_code(self) -> str:
        """Read the boot code from govagn/sitecustomize.py"""
        this_dir = Path(__file__).parent
        sitecustomize_src = this_dir / "govagn" / "sitecustomize.py"

        if sitecustomize_src.exists():
            try:
                # Try to read as UTF-8, but fall back to reading what we can
                return sitecustomize_src.read_text(encoding="utf-8")
            except Exception as e:
                # Fallback to inline code if we can't read the file
                pass

        # Fallback: return inline boot code
        return f"""{self.GUARD_START}
def _boot():
    try:
        from govagn.auto_instrument import AutoInstrumentor
        AutoInstrumentor().run()
    except Exception:
        pass

_boot()
{self.GUARD_END}
"""


# Entry point for setuptools post-install hook
def install():
    """Called by setuptools post-install hook"""
    installer = SitecustomizeInstaller()
    installer.install()


def uninstall():
    """Called by setuptools uninstall hook"""
    installer = SitecustomizeInstaller()
    installer.uninstall()


if __name__ == "__main__":
    # Allow direct invocation for testing
    import sys

    if len(sys.argv) > 1 and sys.argv[1] == "uninstall":
        uninstall()
    else:
        install()
