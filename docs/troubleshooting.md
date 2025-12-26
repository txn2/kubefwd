# Troubleshooting

Common issues and solutions when using kubefwd.

## Permission Errors

### "Permission denied" or "Operation not permitted"

kubefwd requires root privileges. Always use `sudo`:

```bash
# Wrong
kubefwd svc -n default --tui

# Correct
sudo kubefwd svc -n default --tui
```

### KUBECONFIG not found

The `-E` flag preserves environment variables:

```bash
# Wrong - loses KUBECONFIG
sudo kubefwd svc -n default --tui

# Correct - preserves environment
sudo -E kubefwd svc -n default --tui
```

### Custom KUBECONFIG location

If your kubeconfig isn't in the default location:

```bash
export KUBECONFIG=/path/to/custom/config
sudo -E kubefwd svc -n default --tui
```

Or specify directly:

```bash
sudo -E kubefwd svc -n default -c /path/to/custom/config --tui
```

## Connection Issues

### "Connection refused" errors

**Check cluster connectivity:**
```bash
kubectl cluster-info
kubectl get nodes
```

**Verify the context:**
```bash
kubectl config current-context
kubectl config get-contexts
```

**Test a single service:**
```bash
kubectl port-forward svc/my-service 8080:80 -n default
```

### Services not appearing

**Check service exists:**
```bash
kubectl get svc -n default
```

**Verify selector matches:**
```bash
# Your selector
kubectl get svc -n default -l app=api

# If empty, check available labels
kubectl get svc -n default --show-labels
```

**Check RBAC permissions:**
```bash
# Can you list services?
kubectl auth can-i list services -n default

# Can you list/get/watch pods?
kubectl auth can-i list pods -n default
kubectl auth can-i get pods -n default
kubectl auth can-i watch pods -n default
```

### Port forward keeps disconnecting

**Enable auto-reconnect:**
```bash
sudo -E kubefwd svc -n default --tui -a
```

Auto-reconnect is enabled by default in TUI mode.

**Increase timeout:**
```bash
sudo -E kubefwd svc -n default --tui -t 600
```

**Check pod health:**
```bash
kubectl get pods -n default
kubectl describe pod <pod-name> -n default
```

### "No pods found" for a service

Services without running pods can't be forwarded:

```bash
# Check pods for a service
kubectl get endpoints my-service -n default

# If empty, check deployment
kubectl get deployment -n default
kubectl describe deployment my-deployment -n default
```

## Hosts File Issues

### Stale entries after crash

If kubefwd exits unexpectedly, hosts entries may remain:

**Option 1: Purge stale entries**
```bash
sudo -E kubefwd svc -n default -p --tui
```

**Option 2: Restore from backup**
```bash
sudo cp ~/hosts.original /etc/hosts
```

**Option 3: Manual cleanup**
```bash
# View kubefwd entries (IPs in 127.1.x.x - 127.255.x.x range)
grep "^127\.[1-9]" /etc/hosts

# Edit manually
sudo nano /etc/hosts
```

### Backup refresh

If your hosts file has changed and you want a fresh backup:

```bash
sudo -E kubefwd svc -n default -b --tui
```

### Hosts file locked

Another process may be modifying the hosts file:

```bash
# Check for locks (Linux)
sudo lsof /etc/hosts

# On macOS
sudo lsof | grep /etc/hosts
```

## Network Issues

### IP address conflicts

If the default IP range conflicts with your network:

**Use IP reservations:**
```bash
sudo -E kubefwd svc -n default -r my-service.default:127.50.50.1 --tui
```

**Or a config file:**
```yaml
# fwdconf.yml
baseIP: 127.50.50.1
reservations:
  - service: api
    namespace: default
    ip: 127.50.50.1
```

### Port already in use

```bash
# Find what's using the port
sudo lsof -i :8080

# Option 1: Stop the conflicting process
# Option 2: Use port mapping
sudo -E kubefwd svc -n default -m 8080:18080 --tui
```

### DNS not resolving service names

**Check hosts file was updated:**
```bash
grep my-service /etc/hosts
```

**Flush DNS cache:**
```bash
# macOS
sudo dscacheutil -flushcache; sudo killall -HUP mDNSResponder

# Linux (systemd-resolved)
sudo systemd-resolve --flush-caches

# Linux (nscd)
sudo /etc/init.d/nscd restart
```

## TUI Issues

### TUI not displaying correctly

**Check terminal size:**
- Minimum 80x24 recommended
- Resize terminal and restart kubefwd

**Check terminal type:**
```bash
echo $TERM
# Should be xterm-256color, screen-256color, or similar
```

**Try a different terminal:**
- macOS: iTerm2, Terminal.app
- Linux: GNOME Terminal, Konsole
- Windows: Windows Terminal

### Colors not showing

**Enable 256-color support:**
```bash
export TERM=xterm-256color
sudo -E kubefwd svc -n default --tui
```

### Mouse not working

Some SSH connections disable mouse support. Try:
- Enabling mouse support in your SSH client
- Using a local terminal instead

### Keyboard shortcuts not working

**Check for conflicting keybindings:**
- Terminal emulator shortcuts
- tmux/screen bindings
- Shell key bindings

**Try without multiplexer:**
Run kubefwd outside tmux/screen to test.

## Performance Issues

### High CPU usage

**Reduce resync frequency:**
```bash
sudo -E kubefwd svc -n default --resync-interval 10m --tui
```

**Forward fewer services:**
```bash
sudo -E kubefwd svc -n default -l app=needed-only --tui
```

### Slow startup

**Check cluster latency:**
```bash
time kubectl get svc -n default
```

**Forward specific services:**
```bash
sudo -E kubefwd svc -n default -f metadata.name=my-service --tui
```

### Memory usage growing

With many services, memory usage is expected. If excessive:
1. Forward fewer services using selectors
2. Restart kubefwd periodically in scripts

## Platform-Specific Issues

### macOS

**"Operation not permitted" on Big Sur+**

System Integrity Protection may block hosts file modification:
1. This is rare; usually sudo is sufficient
2. If persistent, check SIP status: `csrutil status`

**Network extension issues**

Some VPNs/firewalls conflict with loopback interfaces:
1. Temporarily disable VPN
2. Whitelist 127.x.x.x range in firewall

### Linux

**AppArmor/SELinux blocking**

```bash
# Check SELinux status
getenforce

# Temporarily permissive (for testing)
sudo setenforce 0
```

**systemd-resolved conflicts**

```bash
# Check if resolved manages /etc/hosts
ls -la /etc/hosts

# If symlinked, may need direct modification
```

### Windows

**Run as Administrator**

Right-click terminal → "Run as Administrator"

**Hosts file location**

Windows hosts file is at:
```
C:\Windows\System32\drivers\etc\hosts
```

Specify with `--hosts-path` if needed.

## Getting Help

### Enable verbose logging

```bash
sudo -E kubefwd svc -n default -v --tui
```

This shows:
- Service discovery details
- Pod selection logic
- IP allocation
- Connection lifecycle events

### Check kubefwd version

```bash
kubefwd version
```

### Report issues

Include in bug reports:
1. kubefwd version
2. Kubernetes version (`kubectl version`)
3. OS and version
4. Verbose output (`-v` flag)
5. Steps to reproduce

Report issues at: https://github.com/txn2/kubefwd/issues
