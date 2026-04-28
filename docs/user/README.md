# JVS User Docs

JVS saves real folders as save points. Use these docs when you want to learn
the product workflow rather than the implementation details.

| Start here | Use it for |
| --- | --- |
| [Quickstart](quickstart.md) | First folder, first save point, first restore |
| [Concepts](concepts.md) | Save point vocabulary and how the pieces fit |
| [Command Reference](commands.md) | Flags and command behavior |
| [Examples](examples.md) | Practical workflows |
| [FAQ](faq.md) | Short answers to common questions |
| [Troubleshooting](troubleshooting.md) | Fixing common errors |
| [Safety](safety.md) | What JVS changes, refuses, and protects |
| [Recovery](recovery.md) | Resuming or rolling back an interrupted restore |

The normal flow is:

```text
init -> save -> history -> view -> restore
```

For a second real folder based on an earlier save point, use:

```bash
jvs workspace new <name> --from <save>
```
