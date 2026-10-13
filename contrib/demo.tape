# VHS tape for the README hero GIF.
#
#   vhs contrib/demo.tape
#
# Requirements on PATH: vhs, ttyd, ffmpeg, and the mail_cli binary (with a
# configured account). The recipient autocomplete step needs a contacts.json
# with an entry beginning "alice"; the LLM summary needs a local endpoint
# (`mail_cli config set llm_endpoint ...`).
#
# This is a starting point, not a finished recording. The exact cursor moves
# depend on the account's folder layout, so tune the sequence against your own
# mailbox before committing the GIF. The output lands in docs/assets/demo.gif
# and is deliberately NOT committed or referenced from README yet.

Require mail_cli

Output docs/assets/demo.gif
Set Shell bash
Set FontSize 18
Set Width 1400
Set Height 820
Set Padding 16
Set Margin 24
Set MarginFill "#1e1e2e"
Set BorderRadius 8
Set Theme "Dracula"
Set TypingSpeed 55ms

# Start from a clean screen without recording the clear.
Hide
Type "clear"
Enter
Show

# --- Launch and walk the folder tree ------------------------------------
Type "mail_cli tui"
Sleep 400ms
Enter
Sleep 2500ms

# The tree starts closed: f opens it, j/k walk it, Enter descends into a
# folder. The two j presses below are the part most likely to need tuning.
Type "f"
Sleep 1000ms
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Enter
Sleep 1500ms

# --- Split view ---------------------------------------------------------
Type "j"
Sleep 300ms
Enter
Sleep 1500ms
# p toggles the message preview pane beside the list.
Type "p"
Sleep 1800ms

# --- Recipient autocomplete while composing -----------------------------
Type "c"
Sleep 1200ms
Type "alice"
Tab
Sleep 1500ms
# Escape saves the draft and returns to the browser (q/esc both do).
Escape
Sleep 1000ms

# --- Local LLM summary --------------------------------------------------
# The TUI opens the summary on F4. VHS's command reference has no function-key
# command, so this step cannot be scripted here. Record it interactively with
# `vhs record > contrib/demo.tape` and press F4 by hand, or drop the summary
# from the GIF.
#
# Type "F4"   # not valid VHS

# --- Leave --------------------------------------------------------------
Type "q"
Sleep 1500ms
