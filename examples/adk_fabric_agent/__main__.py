# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Allow running as `python -m adk_scion_agent`.

Wraps the real entry point with crash reporting so that import errors
and early startup failures are written to:
  1. /proc/1/fd/2 (sciontool's stderr → container logs the user can see)
  2. agent-info.json (readable by scion's status system)
  3. The process's own stderr (visible in the tmux pane)
"""

import json
import os
import sys
import traceback


def _log_to_init(message: str) -> None:
    """Write directly to PID 1 (sciontool) stderr so it appears in container logs."""
    try:
        with open("/proc/1/fd/2", "w") as f:
            f.write(message + "\n")
            f.flush()
    except Exception:
        pass


def _report_crash(message: str) -> None:
    """Write crash details to container logs, agent-info.json, and stderr."""
    print(message, file=sys.stderr, flush=True)
    _log_to_init(message)
    try:
        info_path = os.path.join(
            os.environ.get("HOME", "/home/scion"), "agent-info.json"
        )
        with open(info_path, "w") as f:
            json.dump({"activity": "error", "error": message}, f)
    except Exception:
        pass


try:
    from .run import main
except Exception:
    _report_crash(
        f"[adk_scion_agent] Failed to import agent modules:\n"
        f"{traceback.format_exc()}"
    )
    sys.exit(1)

try:
    main()
except Exception:
    _report_crash(
        f"[adk_scion_agent] Agent crashed:\n"
        f"{traceback.format_exc()}"
    )
    sys.exit(1)
