# TCT Scrooper

## Droplet Access
```bash
ssh scrooper-droplet
```

App directory: `/srv/tct_scrooper`

### Commands
```bash
scrooper start|stop|restart|status  # systemd service
scrooper logs                        # journalctl -f
scrooper tui                         # launch TUI
scrooper scrape                      # one-shot scrape
```

### Direct DB access
```bash
cd /srv/tct_scrooper && source .env
psql "$SUPABASE_DB_URL" -c "SELECT ..."
```

### Binaries
- `tct_scrooper` - main daemon
- `tui` - terminal UI
