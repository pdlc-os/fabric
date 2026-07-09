# Chat App and Hub Startup Race Conditions

## Problem Description
During the installation and startup of the Fabric Chat App as a self-managed plugin for the Fabric Hub on a fresh VM, two distinct race conditions prevented the Hub from successfully connecting to and utilizing the Chat App's broker plugin.

### 1. Systemd Service Ordering
The generated `fabric-chat-app.service` systemd unit was configured with `After=fabric-hub.service` and `Wants=fabric-hub.service`. 
However, the `fabric-hub` process connects to the `fabric-chat-app` plugin on port `9090` synchronously during its startup sequence. Because `fabric-hub` does not implement retries for connecting to this self-managed plugin, starting the chat app *after* the hub guaranteed a `connection refused` error. 

### 2. Plugin Callback Wiring Race
Even with the systemd ordering fixed (where `fabric-chat-app` starts first or simultaneously), a secondary race condition existed within the `go-plugin` RPC initialization.
When the Hub establishes the RPC connection to the Chat App, the `go-plugin` framework immediately invokes `SetHostCallbacks` on the Chat App side. The Chat App would instantly attempt to use these callbacks to invoke `RequestSubscription` back against the Hub.
However, the Hub's `HostCallbacksForwarder` does not have its underlying proxy target wired up until slightly later in the Hub's startup sequence. This resulted in the Chat App receiving a `host callbacks not yet available` error and failing to register its subscriptions for Grove messages.

## Proposed Fix
1. **Remove Systemd Dependency:** Modify `extras/fabric-chat-app/install.sh` to remove the `After=fabric-hub.service` and `Wants=fabric-hub.service` directives from the generated `fabric-chat-app.service` unit, allowing it to start independently and be ready when the Hub boots.
2. **Implement Subscription Retry Backoff:** Modify `extras/fabric-chat-app/internal/chatapp/broker.go`. Track subscriptions that were requested before the host callbacks are fully ready. When `SetHostCallbacks` is called, launch a goroutine that attempts to establish the subscriptions. If the call returns `host callbacks not yet available`, implement a loop with a 1-second `time.Sleep` backoff (up to 10 retries) until the Hub's forwarder proxy is successfully wired up.

## Diff of the Fix

```diff
diff --git a/extras/fabric-chat-app/go.mod b/extras/fabric-chat-app/go.mod
index aac6b3d5..9620d820 100644
--- a/extras/fabric-chat-app/go.mod
+++ b/extras/fabric-chat-app/go.mod
@@ -19,7 +19,7 @@ require (
        cloud.google.com/go/compute/metadata v0.9.0 // indirect
        cloud.google.com/go/iam v1.5.3 // indirect
        github.com/cespare/xxhash/v2 v2.3.0 // indirect
-       github.com/fatih/color v1.13.0 // indirect
+       github.com/fatih/color v1.16.0 // indirect
        github.com/felixge/httpsnoop v1.0.4 // indirect
        github.com/go-logr/logr v1.4.3 // indirect
        github.com/go-logr/stdr v1.2.2 // indirect
diff --git a/extras/fabric-chat-app/go.sum b/extras/fabric-chat-app/go.sum
index c0c60dc4..9271886e 100644
--- a/extras/fabric-chat-app/go.sum
+++ b/extras/fabric-chat-app/go.sum
@@ -25,8 +25,9 @@ github.com/envoyproxy/go-control-plane/envoy v1.36.0 h1:yg/JjO5E7ubRyKX3m07GF3re
 github.com/envoyproxy/go-control-plane/envoy v1.36.0/go.mod h1:ty89S1YCCVruQAm9OtKeEkQLTb+Lkz0k8v9W0Oxsv98=
 github.com/envoyproxy/protoc-gen-validate v1.3.0 h1:TvGH1wof4H33rezVKWSpqKz5NXWg5VPuZ0uONDT6eb4=
 github.com/envoyproxy/protoc-gen-validate v1.3.0/go.mod h1:HvYl7zwPa5mffgyeTUHA9zHIH36nmrm7oCbo4YKoSWA=
-github.com/fatih/color v1.13.0 h1:8LOYc1KYPPmyKMuN8QV2DNRWNbLo6LZ0iLs8+mlH53w=
 github.com/fatih/color v1.13.0/go.mod h1:kLAiJbzzSOZDVNGyDpeOxJ47H46qBXwg5ILebYFFOfk=
+github.com/fatih/color v1.16.0 h1:zmkK9Ngbjj+K0yRhTVONQh1p/HknKYSlNT+vZCzyokM=
+github.com/fatih/color v1.16.0/go.mod h1:fL2Sau1YI5c0pdGEVCbKQbLXB6edEj1ZgiY4NijnWvE=
 github.com/felixge/httpsnoop v1.0.4 h1:NFTV2Zj1bL4mc9sqWACXbQFVBBg2W3GPvqp8/ESS2Wg=
 github.com/felixge/httpsnoop v1.0.4/go.mod h1:m8KPJKqk1gH5J9DgRY2ASl2lWCfGKXixSwevea8zH2U=
 github.com/go-jose/go-jose/v4 v4.1.4 h1:moDMcTHmvE6Groj34emNPLs/qtYXRVcd6S7NHbHz3kA=
diff --git a/extras/fabric-chat-app/install.sh b/extras/fabric-chat-app/install.sh
index 9c4680a9..e809c6c3 100755
--- a/extras/fabric-chat-app/install.sh
+++ b/extras/fabric-chat-app/install.sh
@@ -241,8 +241,7 @@ substep "Installing systemd unit"
 cat > "${TMPDIR}/fabric-chat-app.service" <<EOF
 [Unit]
 Description=Fabric Chat App
-After=network.target fabric-hub.service
-Wants=fabric-hub.service
+After=network.target
 
 [Service]
 User=fabric
diff --git a/extras/fabric-chat-app/internal/chatapp/broker.go b/extras/fabric-chat-app/internal/chatapp/broker.go
index dfc2e6e1..8188c12b 100644
--- a/extras/fabric-chat-app/internal/chatapp/broker.go
+++ b/extras/fabric-chat-app/internal/chatapp/broker.go
@@ -21,12 +21,12 @@ import (
        "log/slog"
        "net"
        "sync"
+       "time"
 
        "github.com/pdlc-os/fabric/pkg/messages"
        "github.com/pdlc-os/fabric/pkg/plugin"
        goplugin "github.com/hashicorp/go-plugin"
 )
-
 // MessageHandler is called when a message is received from the Hub via the broker plugin.
 type MessageHandler func(ctx context.Context, topic string, msg *messages.StructuredMessage) error
 
@@ -136,9 +136,35 @@ func (b *BrokerServer) HealthCheck() (*plugin.HealthStatus, error) {
 // SetHostCallbacks is called by the go-plugin framework to provide the reverse channel.
 func (b *BrokerServer) SetHostCallbacks(hc plugin.HostCallbacks) {
        b.mu.Lock()
-       defer b.mu.Unlock()
        b.hostCallbacks = hc
+       subs := make([]string, 0, len(b.subscriptions))
+       for p := range b.subscriptions {
+               subs = append(subs, p)
+       }
+       b.mu.Unlock()
+
        b.log.Info("host callbacks connected")
+
+       go func() {
+               for _, pattern := range subs {
+                       // Retry loop since the host forwarder may not have its underlying implementation set immediately.
+                       for i := 0; i < 10; i++ {
+                               err := hc.RequestSubscription(pattern)
+                               if err == nil {
+                                       b.log.Info("subscribed to deferred pattern", "pattern", pattern)
+                                       break
+                               }
+
+                               if err.Error() == "host callbacks not yet available" {
+                                       b.log.Debug("host callbacks not ready yet, retrying...", "pattern", pattern, "attempt", i+1)
+                                       time.Sleep(time.Second)
+                               } else {
+                                       b.log.Error("failed to request deferred subscription", "pattern", pattern, "error", err)
+                                       break
+                               }
+                       }
+               }
+       }()
 }
 
 // HostCallbacks returns the host callbacks interface (for requesting subscriptions).
@@ -150,6 +176,10 @@ func (b *BrokerServer) HostCallbacks() plugin.HostCallbacks {
 
 // RequestSubscription asks the Hub to subscribe this plugin to a topic pattern.
 func (b *BrokerServer) RequestSubscription(pattern string) error {
+       b.mu.Lock()
+       b.subscriptions[pattern] = true
+       b.mu.Unlock()
+
        hc := b.HostCallbacks()
        if hc == nil {
                return fmt.Errorf("host callbacks not available")
```