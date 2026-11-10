# elelemstream

Bridges [elelem](https://github.com/psyb0t/elelem)'s streaming callbacks to essessey's content block protocol. `Adapter` binds the callbacks to a request and emits correctly indexed thinking, text, tool use and tool result blocks through an `essessey.Publisher`; `MapStopReason` turns an elelem finish reason into a stop reason. It is the only package in the module that imports elelem.

Read the block index arithmetic and its invariants before you "simplify" anything in here: **[elelem adapter](https://psyb0t.github.io/essessey/elelemstream/)**.
