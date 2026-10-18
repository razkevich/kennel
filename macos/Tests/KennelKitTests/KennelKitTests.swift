import Testing
@testable import KennelKit

@Test func ghosttyConfigAndTheme() {
    let theme = """
    background = 282a36
    foreground = f8f8f2
    palette = 1=#ff5555
    """
    let config = """
    # comment
    font-family = "JetBrains Mono"
    font-size = 14
    theme = light:Solarized,dark:Dracula
    cursor-color = #ffb86c
    palette = 2=50fa7b
    """
    let t = GhosttyTheme.parse(config) { name in name == "Dracula" ? theme : nil }
    #expect(t.fontFamily == "JetBrains Mono")
    #expect(t.fontSize == 14)
    #expect(t.background == "#282a36")
    #expect(t.foreground == "#f8f8f2")
    #expect(t.cursor == "#ffb86c")
    #expect(t.palette[1] == "#ff5555")
    #expect(t.palette[2] == "#50fa7b")
}

@Test func laterKeysOverrideTheme() {
    let t = GhosttyTheme.parse("theme = X\nbackground = 000000") { _ in "background = ffffff" }
    #expect(t.background == "#000000")
}

@Test func launchFromPageMessage() {
    let l = Launch(message: ["cwd": "/r", "title": "KALSHI", "command": "claude --resume abc", "session": "abc", "focus": true])
    #expect(l == Launch(cwd: "/r", title: "KALSHI", command: "claude --resume abc", session: "abc", focus: true))
    #expect(Launch(message: ["cwd": "/r"]) == nil)
    #expect(Launch(message: ["command": "x", "session": ""])?.session == nil)
}

@Test func shellArgsRealAndFake() {
    let l = Launch(cwd: "/r", title: "t", command: "claude -n 'it''s'")
    #expect(l.shellArgs(fake: false) == ["-l", "-c", "claude -n 'it''s'"])
    let fake = l.shellArgs(fake: true)
    #expect(fake[0] == "-c")
    #expect(fake[1].contains("would run: claude"))
    #expect(fake[1].hasSuffix("exec cat"))
}

@Test func serverURLFromOutput() {
    let u = serverURL(fromLine: "▼•ᴥ•▼ kennel map at http://127.0.0.1:51234/?t=abc")
    #expect(u?.port == 51234)
    #expect(u?.query == "t=abc")
    #expect(serverURL(fromLine: "indexing 3/10") == nil)
}

@Test func quoting() {
    #expect(shellQuote("it's") == "'it'\\''s'")
    #expect(jsString("a \"b\"\n") == "\"a \\\"b\\\"\\n\"")
}
