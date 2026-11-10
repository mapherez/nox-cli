# NoX CLI

Your Suite, at your fingertips.

NoX CLI is the interactive terminal interface for NoX Suite. It lets you browse connected applications, choose an operation and see its result without configuring a separate client connection for every product.

The interface uses the tools and metadata published by the applications. There is no fixed list of product commands inside the CLI: a conforming product supplies its own operations and appears through Gateway discovery.

## What you can do

| Capability | What it provides |
| --- | --- |
| Browse your apps | Find the products and tools available to your account. |
| Navigate with the keyboard | Use arrow keys, Enter and Escape to move through the interface. |
| Run operations | Select published choices, provide required inputs and confirm actions where requested. |
| Read results | View structured output as readable fields and lists, with separate error information. |
| Manage your connection | Configure a private Gateway address and a Backend personal API key. |
| Keep credentials private | Store the key in the operating system's credential store or use it only for the current session. |
| Maintain the installation | Check for updates and use the supplied update/removal flows. |

Ordinary navigation is local. Network requests load the catalog, resolve published choices and execute operations. A failed connection does not automatically retry an action that might already have changed data.

## How it fits into NoX Suite

CLI connects to Gateway, which routes requests to the participating products. Backend remains the authority for the personal API key and effective access; each product executes its own operations and enforces its domain rules.

The CLI can run on another computer with private LAN or VPN access to Gateway. It does not need Docker or access to the host's product sockets. Public Gateway destinations are outside its supported connection policy.

AI MCP clients connect directly to Gateway. The CLI is the human interface and does not expose an MCP server of its own.

## Credential handling

Saved keys use the native operating-system credential store, not plaintext configuration. Replacing a key updates the same stored entry. The interface does not reveal, copy or export saved keys.

If secure storage is unavailable, you can explicitly choose session-only use. When Gateway rejects a key, Suite operations are blocked until a valid replacement is supplied. The CLI learns about that rejection through a Gateway exchange; it does not claim immediate notification while idle.

For architecture, storage paths, connection policy, credential ownership and installation recovery, see the [technical reference](TECHNICAL.md).

## Usage

Installation uses a published release and the supplied [macOS/Linux installer](install.sh) or [Windows installer](install.ps1). Installer hosting is managed by the repository's [publication workflow](.github/workflows/installers.yml). End users do not need Go, Node, Python or Docker.

After installation, start the application from your terminal:

```sh
nox
```

In **Settings**, enter your Gateway's private base address and your Backend personal API key, then select **Save and test**. No installation-specific address is preconfigured. Open **Apps** to browse the discovered products.

Use **Up/Down** to select an item and **Enter** to open it. Required choices and confirmations come from the product's published definition. **Enter** or **Esc** returns from a completed result without running the command again; the footer shows the controls for the current screen.

Use **Settings → Updates and installation** for update controls and removal instructions. The supplied [macOS/Linux uninstaller](uninstall.sh) and [Windows uninstaller](uninstall.ps1) can preserve settings or explicitly remove the CLI's configuration and saved credential. Installation and update files are managed in the application's own `install/` directory.

To run from a source checkout, with Go installed:

```sh
go run ./cmd/nox
```

Maintainers can run `npm run release` from a clean, committed checkout to prepare and push a release request. Node.js is needed for that helper only; build and publication run through the existing GitHub Actions workflows.

## License & Attribution

This project is licensed under the **Creative Commons Attribution-NonCommercial-ShareAlike 4.0 International (CC BY-NC-SA 4.0)** license.

### How to Credit This Project

If you use, fork, or modify this code, you **must** provide attribution by linking back to my GitHub profile. Please include the following credit notice prominently in your repository or project documentation:

> **Based on [nox-cli](https://github.com/mapherez/nox-cli) by [mapherez](https://github.com/mapherez)**
> This project includes code from [nox-cli](https://github.com/mapherez/nox-cli), used under the CC BY-NC-SA 4.0 license.

---

*For more details on what you can and cannot do, please refer to the [LICENSE](LICENSE) file in this repository.*

## Disclaimer of Warranty

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
