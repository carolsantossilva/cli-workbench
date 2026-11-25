"""Leitura do comando pelo hook de review antes do push (sem chamar o revisor)."""

import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

HOOK = Path(__file__).with_name("review-before-push.py")
spec = importlib.util.spec_from_file_location("review_before_push", HOOK)
hook = importlib.util.module_from_spec(spec)
spec.loader.exec_module(hook)

COMMIT_WITH_HEREDOC = """git commit -q -F - <<'EOF'
fix: the new run's audit

Mentions git push origin HEAD in the body.
EOF
"""


def git(repo, *args):
    subprocess.run(["git", "-C", str(repo), *args], check=True, capture_output=True)


class PushRequest(unittest.TestCase):
    def test_simple_push(self):
        repo, args = hook.push_request("git push origin HEAD", "/tmp")
        self.assertEqual(args, ["origin", "HEAD"])
        self.assertEqual(repo, Path("/tmp"))

    def test_not_a_push(self):
        self.assertIsNone(hook.push_request("git status", "/tmp"))
        self.assertIsNone(hook.push_request("echo 'git push'", "/tmp"))

    def test_heredoc_with_apostrophe_is_not_a_push(self):
        self.assertIsNone(hook.push_request(COMMIT_WITH_HEREDOC, "/tmp"))

    def test_push_after_heredoc_is_still_seen(self):
        with self.assertRaisesRegex(RuntimeError, "comando separado"):
            hook.push_request(COMMIT_WITH_HEREDOC + "git push origin HEAD\n", "/tmp")

    def test_unbalanced_quote(self):
        self.assertIsNone(hook.push_request("echo it's fine", "/tmp"))
        with self.assertRaisesRegex(RuntimeError, "aspas sem par"):
            hook.push_request('git push origin "HEAD', "/tmp")

    def test_tilde_in_dash_c(self):
        repo, _ = hook.push_request("git -C ~/projeto push", "/tmp")
        self.assertEqual(repo, Path.home() / "projeto")

    def test_compound_and_wrappers_are_refused(self):
        with self.assertRaisesRegex(RuntimeError, "comando separado"):
            hook.push_request("cd x && git push", "/tmp")
        with self.assertRaisesRegex(RuntimeError, "fora de sh/bash"):
            hook.push_request("bash -c 'git push'", "/tmp")
        with self.assertRaisesRegex(RuntimeError, "opções globais"):
            hook.push_request("git -c core.hooksPath=/x push", "/tmp")


class MoreParsing(unittest.TestCase):
    def test_newline_separates_commands(self):
        with self.assertRaisesRegex(RuntimeError, "comando separado"):
            hook.push_request("git push origin --delete feat/x\ngit push origin feat", "/tmp")
        with self.assertRaisesRegex(RuntimeError, "comando separado"):
            hook.push_request("git status\ngit push origin HEAD", "/tmp")

    def test_single_push_with_trailing_newline_or_continuation(self):
        self.assertEqual(hook.push_request("git push origin HEAD\n", "/tmp")[1], ["origin", "HEAD"])
        self.assertEqual(hook.push_request("git push \\\n  origin HEAD", "/tmp")[1], ["origin", "HEAD"])

    def test_newline_inside_quotes_is_text(self):
        self.assertIsNone(hook.push_request('git commit -m "a\ngit push origin HEAD"', "/tmp"))

    def test_here_string_and_unterminated_heredoc_do_not_hide_a_push(self):
        with self.assertRaisesRegex(RuntimeError, "comando separado"):
            hook.push_request("cat <<<EOF\ngit push origin HEAD", "/tmp")
        with self.assertRaisesRegex(RuntimeError, "Execute git push|comando separado"):
            hook.push_request('echo "a <<EOF"\ngit push origin HEAD', "/tmp")


class NestedShells(unittest.TestCase):
    def test_shell_flags_with_c(self):
        for command in ("bash -lc 'git push origin HEAD'", "sh -ec 'git push'", "bash -o pipefail -c 'git push'", "/bin/zsh -c 'cd x && git push'"):
            with self.assertRaisesRegex(RuntimeError, "fora de sh/bash -c", msg=command):
                hook.push_request(command, "/tmp")
        self.assertIsNone(hook.push_request("bash -lc 'npm test'", "/tmp"))

    def test_heredoc_fed_to_a_shell_is_a_script(self):
        for command in ("bash <<'EOF'\ngit push origin HEAD\nEOF", "cat <<EOF | sh\ncd x\ngit -C y push\nEOF"):
            with self.assertRaisesRegex(RuntimeError, "heredoc para o shell", msg=command):
                hook.push_request(command, "/tmp")
        self.assertIsNone(hook.push_request("bash <<'EOF'\nnpm test\nEOF", "/tmp"))
        self.assertIsNone(hook.push_request(COMMIT_WITH_HEREDOC, "/tmp"), "texto de commit continua sendo texto")


class Deletion(unittest.TestCase):
    """Remoção conferida contra um remoto de verdade (um repositório bare local)."""

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        base = Path(self.tmp.name)
        self.remote = base / "remote.git"
        self.repo = base / "clone"
        git(base, "init", "-q", "--bare", "-b", "trunk", str(self.remote))
        git(base, "init", "-q", "-b", "trunk", str(self.repo))
        git(self.repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "x")
        git(self.repo, "remote", "add", "origin", str(self.remote))
        for ref in ("trunk", "HEAD:main", "HEAD:feat/x", "HEAD:fix/y", "HEAD:amb", "HEAD:refs/tags/amb", "HEAD:refs/tags/v1"):
            git(self.repo, "push", "-q", "origin", ref)

    def tearDown(self):
        self.tmp.cleanup()

    def test_delete_branches(self):
        self.assertEqual(hook.deleted_branches(self.repo, ["origin", "--delete", "feat/x", "refs/heads/fix/y"]), ["feat/x", "fix/y"])
        self.assertEqual(hook.deleted_branches(self.repo, ["-d", "origin", "feat/x"]), ["feat/x"])
        self.assertEqual(hook.deleted_branches(self.repo, ["origin", ":feat/x"]), ["feat/x"])

    def test_code_push_is_not_a_deletion(self):
        self.assertIsNone(hook.deleted_branches(self.repo, ["origin", "HEAD"]))
        self.assertIsNone(hook.deleted_branches(self.repo, ["-u", "origin", "HEAD:feat/x"]))
        self.assertIsNone(hook.deleted_branches(self.repo, []))

    def test_main_branch_is_protected(self):
        for args in (["origin", "--delete", "main"], ["origin", ":master"], ["origin", "--delete", "feat/x", "trunk"]):
            with self.assertRaisesRegex(RuntimeError, "principal"):
                hook.deleted_branches(self.repo, args)

    def test_other_spellings_and_tags_are_refused(self):
        for name in ("heads/main", "refs/tags/v1", "tags/v1", "remotes/origin/main", "refs/heads/heads/main", "HEAD"):
            with self.assertRaisesRegex(RuntimeError, "pelo nome"):
                hook.deleted_branches(self.repo, ["origin", "--delete", name])
        with self.assertRaisesRegex(RuntimeError, "não é uma branch"):
            hook.deleted_branches(self.repo, ["origin", "--delete", "v1"])
        with self.assertRaisesRegex(RuntimeError, "ambíguo"):
            hook.deleted_branches(self.repo, ["origin", "--delete", "amb"])

    def test_bad_options_and_unreachable_remote(self):
        with self.assertRaisesRegex(RuntimeError, "sem outras opções"):
            hook.deleted_branches(self.repo, ["origin", "--delete", "--force", "feat/x"])
        with self.assertRaisesRegex(RuntimeError, "sem outras opções"):
            hook.deleted_branches(self.repo, ["--force", "origin", ":feat/x"])
        with self.assertRaisesRegex(RuntimeError, "--delete <branch>"):
            hook.deleted_branches(self.repo, ["origin", "--delete"])
        with self.assertRaisesRegex(RuntimeError, "não foi conferida"):
            hook.deleted_branches(self.repo, ["nao-existe", "--delete", "feat/x"])

    def run_hook(self, command):
        event = {"tool_input": {"command": command}, "cwd": "/"}
        r = subprocess.run([sys.executable, str(HOOK), "claude"], input=json.dumps(event), capture_output=True, text=True, check=True)
        return json.loads(r.stdout)["hookSpecificOutput"]

    def test_hook_lets_deletion_through_without_review(self):
        out = self.run_hook(f"git -C {self.repo} push origin --delete feat/x")
        self.assertNotIn("permissionDecision", out)
        self.assertIn("só de remoção (feat/x)", out["additionalContext"])

    def test_hook_blocks_main_deletion(self):
        self.assertEqual(self.run_hook(f"git -C {self.repo} push origin --delete heads/main")["permissionDecision"], "deny")
        self.assertEqual(self.run_hook(f"git -C {self.repo} push origin --delete feat/x\ngit -C {self.repo} push origin trunk")["permissionDecision"], "deny")


if __name__ == "__main__":
    unittest.main()
