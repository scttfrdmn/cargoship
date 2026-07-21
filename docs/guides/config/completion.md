# Shell completion

CargoShip ships tab-completion for Bash, Zsh, fish, and PowerShell. With it
enabled, you get completion of subcommands, flags, and flag values as you type.

If you installed CargoShip from the Homebrew or `.deb` / `.rpm` packages,
completion is already wired up — skip to [verifying](#verify). If you installed
another way, generate the completion script for your shell with the built-in
`cargoship completion` command.

```bash
cargoship completion --help          # list supported shells
cargoship completion bash --help     # per-shell instructions
```

## Bash

```bash
# Try it in the current shell
source <(cargoship completion bash)

# Install permanently (Linux)
cargoship completion bash | sudo tee /etc/bash_completion.d/cargoship > /dev/null

# Install permanently (macOS, Homebrew bash-completion)
cargoship completion bash > "$(brew --prefix)/etc/bash_completion.d/cargoship"
```

Requires the `bash-completion` package to be installed and sourced from your
`~/.bashrc`.

## Zsh

```bash
# Enable completion if you haven't already
echo "autoload -U compinit; compinit" >> ~/.zshrc

# Install the completion script
cargoship completion zsh > "${fpath[1]}/_cargoship"
```

Start a new shell afterward. On macOS with Homebrew, `${fpath[1]}` is typically
under `$(brew --prefix)/share/zsh/site-functions`.

## fish

```bash
cargoship completion fish > ~/.config/fish/completions/cargoship.fish
```

## PowerShell

```powershell
cargoship completion powershell | Out-String | Invoke-Expression

# To persist, add that line to your PowerShell profile:
cargoship completion powershell >> $PROFILE
```

## Verify

Open a new shell and type a partial command followed by <kbd>Tab</kbd>:

```bash
cargoship up<Tab>          # completes to: cargoship upload
cargoship upload --sh<Tab> # suggests --shard-count, --shard-strategy
```

## Best practices

::: tip
- **Start a fresh shell** after installing — completion loads at shell startup, so
  it won't appear in the session where you generated it.
- **Regenerate after upgrades** so newly added commands and flags complete.
- **Use package installs** (Homebrew / deb / rpm) when you can — completion is set
  up for you and stays current with the binary.
:::

## See also

- [Config files & precedence](/guides/config/files) — persist your common flags instead of retyping.
- Reference: [Configuration & context commands](/reference/commands/config).
