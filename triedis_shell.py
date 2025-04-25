#!/usr/bin/env python3
"""
trie_shell.py — a tiny interactive client for the pytricia-backed Triedis server.

It works like a very cut-down `redis-cli`, letting you type commands such as:

    triedis> SET 192.168.0.0/16 private
    OK
    triedis> GET 192.168.1.15
    "private"
    triedis> DBSIZE
    (integer) 1

Installation
------------
$ pip install redis readline                     # readline is stdlib on *nix; "pyreadline3" on Windows

Usage
-----
$ python trie_shell.py                   # connects to localhost:6379
$ python trie_shell.py -h 10.0.0.5 -p 6380

Type "quit" or Ctrl-D to exit.

Limitations: the shell only supports RESP2 replies and prints them in a
redis-cli-like style (simple strings, integers, arrays, nil). PIPELINEing and
Lua scripting are out of scope for this lightweight helper.
"""

import argparse
import shlex
import sys
from typing import Any

try:
    import readline  # noqa: F401 — line editing & history on POSIX
except ImportError:
    # Windows: `pip install pyreadline3`
    pass

import redis

PROMPT = "triedis> "
HISTORY_FILE = "~/.triedis_history"


def format_resp(obj: Any, indent: int = 0) -> str:
    """Pretty-print Redis responses similar to the official CLI."""
    pad = " " * indent
    if obj is None:
        return f"{pad}(nil)"
    if isinstance(obj, bytes):
        obj = obj.decode()
    if isinstance(obj, str):
        return f"{pad}{obj if obj.startswith('OK') else f'"{obj}"'}"
    if isinstance(obj, int):
        return f"{pad}(integer) {obj}"
    if isinstance(obj, (list, tuple)):
        lines = []
        for i, item in enumerate(obj, 1):
            prefix = f"{pad}{i}) " if indent == 0 else pad
            lines.append(prefix + format_resp(item, indent + (0 if indent == 0 else 2)).lstrip())
        return "\n".join(lines)
    return f"{pad}{obj}"


def main() -> None:
    ap = argparse.ArgumentParser(description="Interactive shell for Triedis")
    ap.add_argument("-H", "--host", default="127.0.0.1", help="host (default 127.0.0.1)")
    ap.add_argument("-p", "--port", type=int, default=6379, help="port (default 6379)")
    args = ap.parse_args()

    try:
        r = redis.Redis(host=args.host, port=args.port, decode_responses=False)
        # Simple connectivity test
        r.ping()
    except Exception as e:
        sys.exit(f"Cannot connect to {args.host}:{args.port} — {e}")

    # Load command history if possible
    try:
        import os, pathlib, readline  # noqa: E401, F401

        hist_path = pathlib.Path(HISTORY_FILE).expanduser()
        if hist_path.exists():
            readline.read_history_file(hist_path)
    except Exception:
        pass

    print(f"Connected to Triedis at {args.host}:{args.port}. Type 'quit' or Ctrl-D to exit.")

    while True:
        try:
            line = input(PROMPT)
        except (EOFError, KeyboardInterrupt):
            print()
            break
        line = line.strip()
        if not line:
            continue
        if line.lower() in {"quit", "exit"}:
            break
        try:
            argv = shlex.split(line)
        except ValueError as ve:
            print(f"(error) {ve}")
            continue
        cmd = [argv[0].upper(), *argv[1:]]
        try:
            resp = r.execute_command(*cmd)
            print(format_resp(resp))
        except Exception as e:
            print(f"(error) {e}")

    # Save history
    try:
        import pathlib, readline  # noqa: F401

        hist_path = pathlib.Path(HISTORY_FILE).expanduser()
        hist_path.parent.mkdir(parents=True, exist_ok=True)
        readline.write_history_file(hist_path)
    except Exception:
        pass


if __name__ == "__main__":
    main()
