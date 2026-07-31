from __future__ import annotations

import importlib
import os
import sys


ENTRY_POINTS = {
    "frida": ("frida_tools.repl", "main"),
    "frida-ps": ("frida_tools.ps", "main"),
    "frida-ls-devices": ("frida_tools.lsd", "main"),
}


def main() -> int:
    if len(sys.argv) < 2:
        print("missing Frida tool name", file=sys.stderr)
        return 2
    tool = os.path.basename(sys.argv[1]).lower().removesuffix(".exe")
    target = ENTRY_POINTS.get(tool)
    if target is None:
        print(f"unsupported bundled Frida tool: {tool}", file=sys.stderr)
        return 2
    sys.argv = [tool, *sys.argv[2:]]
    module = importlib.import_module(target[0])
    result = getattr(module, target[1])()
    return int(result) if isinstance(result, int) else 0


if __name__ == "__main__":
    raise SystemExit(main())
