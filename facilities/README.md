# Facilities

Optional building blocks for storm test suites. `facilities` is its own Go
module, so importing it does not add its dependencies to storm itself.

| Package | Purpose |
| --- | --- |
| `iso` | Build small data ISOs, for example seed volumes a guest finds by label |
| `ssh` | Ephemeral keys, wait for a freshly booted machine, run commands, fetch files |
| `ado` | Azure DevOps job matrices and `setvariable` commands |

The packages are deliberately concrete: consumers define the small interfaces
they need.
