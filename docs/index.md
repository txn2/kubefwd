---
title: kubefwd
description: Bulk port forward Kubernetes services for local development
hide:
  - navigation
  - toc
---

<div class="home-hero">
  <img src="images/brand-text-800-blk.png" alt="kubefwd" class="hero-logo hero-logo-light">
  <img src="images/brand-text-800.png" alt="kubefwd" class="hero-logo hero-logo-dark">
</div>

<div class="home-intro">
  <p><strong>kubefwd</strong> enables developers to work locally while accessing services running in Kubernetes. Connect to <code>db:5432</code>, <code>auth:443</code>, <code>redis:6379</code>, all by service name, exactly as in-cluster. No environment config, no Docker Compose. Just run kubefwd.</p>
</div>

<div class="home-buttons">
  <a href="getting-started/" class="btn btn-primary">Get Started</a>
  <a href="https://github.com/txn2/kubefwd" class="btn btn-secondary">GitHub</a>
</div>

<div class="home-screenshot">
  <img src="images/tui-110-main-active.png" alt="kubefwd TUI">
</div>

<div class="home-features">
  <div class="feature">
    <span class="feature-icon">🔌</span>
    <div>
      <strong>Unique IP per Service</strong>
      <span>Each service gets its own 127.x.x.x. Multiple databases on port 3306? No conflicts.</span>
    </div>
  </div>
  <div class="feature">
    <span class="feature-icon">🔄</span>
    <div>
      <strong>Auto-Reconnect</strong>
      <span>Pods restart? kubefwd reconnects automatically with exponential backoff.</span>
    </div>
  </div>
  <div class="feature">
    <span class="feature-icon">📊</span>
    <div>
      <strong>Interactive TUI</strong>
      <span>Real-time status, traffic metrics, and pod logs in your terminal.</span>
    </div>
  </div>
  <div class="feature">
    <span class="feature-icon">🌐</span>
    <div>
      <strong>Service Names Work</strong>
      <span>Updates /etc/hosts so any app can access services by name.</span>
    </div>
  </div>
</div>

<div class="home-install">

```bash
# macOS
brew install txn2/tap/kubefwd

# Then forward services
sudo -E kubefwd svc -n my-namespace --tui
```

</div>

<div class="home-footer">
  <a href="https://github.com/txn2/kubefwd/releases"><img src="https://img.shields.io/github/release/txn2/kubefwd.svg?style=flat-square" alt="Release"></a>
  <a href="https://github.com/txn2/kubefwd"><img src="https://img.shields.io/github/stars/txn2/kubefwd?style=flat-square" alt="Stars"></a>
  <p>Open source by <a href="https://twitter.com/cjimti">Craig Johnston</a></p>
</div>
