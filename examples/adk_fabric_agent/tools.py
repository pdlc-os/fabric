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

"""Agent-callable tools for scion status signaling.

File and shell operations are provided by ADK's EnvironmentToolset
(via LocalEnvironment). This module exposes only the scion-specific
sciontool_status tool.
"""

import logging

from . import sciontool

logger = logging.getLogger(__name__)


def sciontool_status(status_type: str, message: str) -> dict:
    """Signal a lifecycle event to scion's orchestration layer.

    Args:
        status_type: One of "ask_user", "blocked", "task_completed", or
            "limits_exceeded".
        message: A description of the event (task summary, question, or reason).

    Returns:
        A dict confirming the status update.
    """
    valid_types = {"ask_user", "blocked", "task_completed", "limits_exceeded"}
    if status_type not in valid_types:
        return {
            "status": "error",
            "message": (
                f"Invalid status_type '{status_type}'. "
                f"Must be one of: {', '.join(sorted(valid_types))}"
            ),
        }

    sciontool.run_status(status_type, message)

    return {
        "status": "success",
        "message": f"Reported {status_type}: {message}",
    }
