# NoX CLI

Your Suite, at your fingertips.

NoX CLI brings your NoX apps into one interactive terminal. Browse your apps, choose what you want to do, select the relevant item and read the result—all from the keyboard.

- One connection for the apps in your Suite.
- Arrow-key navigation, searchable menus and readable results.
- Choices supplied by each app, with confirmation when an operation requires it.
- API keys stored in your operating system's credential store, with a session-only option.
- Update controls inside the app.

Runs on **Windows, macOS and Linux**, on Intel/AMD and ARM64 computers. You do not need Go, Node.js, Python or Docker to use it.

## Before you start

You need a running NoX Suite installation, your **Gateway address** and a **personal API key from Backend**. Your computer must be able to reach Gateway through your private network or VPN, such as Tailscale or Twingate. Public Gateway addresses are not supported.

NoX CLI is for interactive use. To connect an AI assistant to your Suite instead, configure that assistant's MCP connection directly to Gateway.

[Technical reference](TECHNICAL.md) · [Report a problem](https://github.com/mapherez/nox-cli/issues)

## Usage

### Installation

Open a terminal and run the command for your operating system. The installer downloads the latest published release, selects the correct executable and adds `nox` to your user PATH. Administrator access is not required.

**Windows — PowerShell**

```powershell
& ([scriptblock]::Create((Invoke-RestMethod 'https://github.com/mapherez/nox-cli/releases/latest/download/install.ps1')))
```

**macOS / Linux — Terminal**

```sh
curl -fsSL https://github.com/mapherez/nox-cli/releases/latest/download/install.sh | sh
```

Open a **new terminal** after installation, then start NoX:

```sh
nox
```

### Connect to your Suite

1. Open **Settings**.
2. Enter the **Gateway base address** used on your private network, without `/mcp`.
3. Enter your **Backend personal API key** and choose **Save and test**. The key is hidden and is not stored in a plain-text configuration file.
4. Open **Apps** and choose an application.

If your system's credential store is unavailable, you can select **Use API key only for this session**. You will need to enter it again after closing NoX.

Use **Up/Down** to move, **Enter** to select and **Esc** to go back. The footer shows the controls available on the current screen. When an operation finishes, **Enter** or **Esc** returns without running it again. Press **R** to refresh the app catalog.

### Update

Open **Settings → Updates and installation → Check for updates**, then choose **Install update** when one is available. Confirm the update and start `nox` again afterward. Your settings and saved API key are preserved.

### Uninstall

Close NoX before running the command for your operating system. These commands remove the installed application and its own PATH changes, while **keeping your settings and saved key**.

**Windows — PowerShell**

```powershell
& ([scriptblock]::Create((Invoke-RestMethod 'https://github.com/mapherez/nox-cli/releases/latest/download/uninstall.ps1')))
```

**macOS / Linux — Terminal**

```sh
curl -fsSL https://github.com/mapherez/nox-cli/releases/latest/download/uninstall.sh | sh
```

To also delete the CLI's settings and saved API key, use the corresponding command below instead.

**Windows — PowerShell**

```powershell
& ([scriptblock]::Create((Invoke-RestMethod 'https://github.com/mapherez/nox-cli/releases/latest/download/uninstall.ps1'))) -Purge
```

**macOS / Linux — Terminal**

```sh
curl -fsSL https://github.com/mapherez/nox-cli/releases/latest/download/uninstall.sh | sh -s -- --purge
```

Open a new terminal after removal. Other NoX apps and their data are unaffected. Downloads and update preparation files stay inside the CLI's own `install/` directory and are cleaned up afterward.

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
