---
title: Apple Container DNS Setup
description: Manual steps to configure DNS for Apple Container runtime on macOS.
---

When using Apple Container as your container runtime, Fabric agents need to reach the Hub server. This requires a DNS rule that maps `host.containers.internal` to the loopback address.

Apple Container's DNS rules persist across sessions but the underlying PF (packet filter) rules do not survive macOS reboots. You need to re-run this command after each reboot:

```bash
sudo container system dns create host.containers.internal --localhost 203.0.113.1
```

## Why sudo is required

The DNS setup modifies macOS PF (packet filter) rules, which require root access. This is an Apple Container limitation — there is no rootless alternative for PF rules.

## Automating after reboot

Because the command requires root, it must be installed as a **system-level LaunchDaemon** (not a user LaunchAgent). User-level agents run without a terminal, so `sudo` cannot prompt for a password.

Create `/Library/LaunchDaemons/org.fabric.apple-container-dns.plist` as root:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>org.fabric.apple-container-dns</string>
    <key>ProgramArguments</key>
    <array>
        <string>/opt/homebrew/bin/container</string>
        <string>system</string>
        <string>dns</string>
        <string>create</string>
        <string>host.containers.internal</string>
        <string>--localhost</string>
        <string>203.0.113.1</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
</dict>
</plist>
```

:::note
Adjust the path `/opt/homebrew/bin/container` if Apple Container is installed elsewhere on your system.
:::

Load the daemon (runs immediately and on every subsequent boot):

```bash
sudo launchctl bootstrap system /Library/LaunchDaemons/org.fabric.apple-container-dns.plist
```

## Verification

To check if the DNS rule is configured:

```bash
container system dns list
```

You should see `host.containers.internal` in the output.
