# RSSGrid

A simple, fast, and persistent personal dashboard for consuming text-based content from various web feeds.

## Features

- OIDC authentication for secure access
- Grid-based feed layout
- Mark posts as read/unread
- Feed management (add/remove feeds)
- Automatic feed updates
- Clean, minimalist UI

## Requirements

- Go 1.21 or later
- SQLite 3
- An OIDC provider (e.g., Google, Auth0, etc.)

## Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/boris/go-rssgrid.git
   cd go-rssgrid
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

## Usage

1. Visit `http://localhost:8080` in your browser
2. Log in using your OIDC provider
3. Add feeds through the settings page
4. View your feeds in the grid layout
5. Click on posts to mark them as read

## Development

The project structure is organized as follows:

- `cmd/rssgrid/`: Main application entry point
- `internal/`: Internal packages
  - `config/`: Configuration management using fig
  - `db/`: Database operations (with embedded migrations)
  - `feed/`: Feed fetching and parsing
  - `server/`: HTTP server and handlers
  - `templates/`: HTML templates
- `config/`: Example configuration files

## License

This project is licensed under the MIT License - see the LICENSE file for details.
