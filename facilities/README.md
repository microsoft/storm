# Facilities

Optional building blocks for storm test suites. `facilities` is its own Go
module, so importing it does not add its dependencies to storm itself.

| Package | Purpose |
| --- | --- |
| `iso` | Build small data ISOs, for example seed volumes a guest finds by label |
| `ssh` | Ephemeral keys, readiness, buffered/streaming commands, file transfers |
| `ado` | Azure DevOps job matrices and `setvariable` commands |

The packages are deliberately concrete: consumers define the small interfaces
they need.

## SSH

`Dial` and `WaitReady` contexts only bound connection establishment; a connected
client remains usable after those contexts end. `WaitDisconnected(ctx)` waits
for an actual connection closure without probing or reconnecting.

`Run` buffers stdout/stderr. `RunIO(ctx, command, stdin, stdout, stderr)` streams
them instead, leaving the result's output fields empty; stdout/stderr may share
a writer. Nonzero exit statuses are results, not transport errors. Unknown exit
statuses remain `-1`, and wrapped errors retain their types for `errors.As`.
Commands are never retried.

`Fetch(ctx, remotePath, dst)` and `Upload(ctx, remotePath, src)` use SFTP.
Set `Config.SFTPCommand` to run a custom server command, such as a passwordless
sudo invocation; otherwise the standard subsystem is used. Uploads stage files
privately with mode `0600`; replacing an existing file requires the server's
POSIX rename extension. File and SFTP close errors are returned.

Cancellation covers session/command/SFTP startup and transfers without closing
unrelated sessions. Pending channel opens are released when the server responds
or the client closes. Caller streams are never closed; writers must not block
indefinitely, and callers must unblock pending stdin/source reads on cancellation.

`GenerateKey` creates ed25519 keys. Check guest crypto-policy support, or supply
another `ssh.Signer` through `Config.Auth`.
