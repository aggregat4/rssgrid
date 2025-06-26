# RSSGrid

A simple, fast, and persistent personal dashboard for consuming text-based content from various web feeds.

## Requirements

- Go 1.21 or later
- SQLite 3
- An OIDC provider

## Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/aggregat4/rssgrid.git
   cd rssgrid
   ```

2. Install dependencies:
   ```bash
   go mod download
   ```

3. Set up configuration:
   ```bash
   # Copy the example configuration file
   mkdir -p ~/.config/rssgrid
   cp config/rssgrid.json.example ~/.config/rssgrid/rssgrid.json
   
   # Edit the configuration file with your settings
   nano ~/.config/rssgrid/rssgrid.json
   ```

4. Build and run:
   ```bash
   go build -o rssgrid cmd/rssgrid/main.go
   ./rssgrid
   ```

## Configuration

RSSGrid uses a configuration file located at `~/.config/rssgrid/rssgrid.json` (or `$XDG_CONFIG_HOME/rssgrid/rssgrid.json` if set). You can also use environment variables for sensitive configuration.

### Configuration File

The configuration file uses JSON format. Here's an example:

```json
{
  "addr": ":8080",
  "db_path": "rssgrid.db",
  "update_interval": "30m",
  "session_key": "your-secure-session-key",
  "oidc": {
    "issuer_url": "https://your-oidc-provider.com",
    "client_id": "your-client-id",
    "client_secret": "your-client-secret",
    "redirect_url": "http://localhost:8080/auth/callback"
  }
}
```

### Environment Variables

For sensitive configuration, you can use environment variables instead of putting them in the config file:

- `RSSGRID_OIDC_ISSUER_URL`: The URL of your OIDC provider
- `RSSGRID_OIDC_CLIENT_ID`: Your OIDC client ID
- `RSSGRID_OIDC_CLIENT_SECRET`: Your OIDC client secret
- `RSSGRID_SESSION_KEY`: A secure key for session encryption

Environment variables take precedence over values in the configuration file.
