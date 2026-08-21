#!/usr/bin/env python3
import argparse
import runpy
import sys
import urllib.request


def main():
    ap = argparse.ArgumentParser(add_help=False)
    ap.add_argument("--client-timeout-floor", type=float, required=True)
    ap.add_argument("--script", required=True)
    known, passthrough = ap.parse_known_args()

    original_urlopen = urllib.request.urlopen
    stats = {"eligible_calls": 0, "timeouts_raised": 0, "max_original_timeout": 0.0}

    def safe_urlopen(url, data=None, timeout=urllib.request.socket._GLOBAL_DEFAULT_TIMEOUT, *, context=None):
        target = getattr(url, "full_url", str(url))
        adjusted = timeout
        local_llm = target.startswith("http://127.0.0.1:9090") or target.startswith("http://localhost:9090")
        if local_llm:
            stats["eligible_calls"] += 1
            if isinstance(timeout, (int, float)):
                stats["max_original_timeout"] = max(stats["max_original_timeout"], float(timeout))
                if float(timeout) < known.client_timeout_floor:
                    adjusted = known.client_timeout_floor
                    stats["timeouts_raised"] += 1
            else:
                adjusted = known.client_timeout_floor
                stats["timeouts_raised"] += 1
        if context is None:
            return original_urlopen(url, data=data, timeout=adjusted)
        return original_urlopen(url, data=data, timeout=adjusted, context=context)

    urllib.request.urlopen = safe_urlopen
    sys.argv = [known.script] + passthrough
    print(
        f"a13_transport_runner=ACTIVE client_timeout_floor_s={known.client_timeout_floor} "
        f"script={known.script}",
        flush=True,
    )
    try:
        runpy.run_path(known.script, run_name="__main__")
    finally:
        print(
            "a13_transport_runner_stats="
            f"eligible_calls={stats['eligible_calls']} "
            f"timeouts_raised={stats['timeouts_raised']} "
            f"max_original_timeout_s={stats['max_original_timeout']}",
            flush=True,
        )


if __name__ == "__main__":
    main()
