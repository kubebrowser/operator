## About

This operator integrates a full web-browsing capability directly into your cluster.
Without it, testing a service typically requires creating a `Route`, exec'ing into another pod and running `curl`, or setting up a port-forward. Now you can simply open a browser from inside the cluster to:

- Test services from within the cluster
- Validate network connectivity across namespaces
- Verify access to external services
- Debug routing, DNS, and access issues directly from the cluster's perspective

## Features

- A `Browser` custom resource that deploys a Chromium browser inside a pod
- A REST API for remote browser control
- A **Networking -> Browsers** page in the web console with VNC access to your browsers

## Installing

After installing the operator, create a `BrowserSystem` instance. Configure the fields in `.spec` needed, otherwise leave it blank for defaults.

```yaml
apiVersion: core.kubebrowser.io/v1alpha1
kind: BrowserSystem
metadata:
  name: browsersystem
  namespace: kubebrowser
spec:
  enableApiService: true
  enableConsolePlugin: true
```

## Usage

Once the operator is installed and the `BrowserSystem` is deployed, you may create a `Browser` resource.

```yaml
apiVersion: core.kubebrowser.io/v1alpha1
kind: Browser
metadata:
  name: mybrowser
spec:
  started: true
```

`spec.started` controls whether the browser is running (it defaults to `true`). Set it to `false` to stop the browser pod without deleting the `Browser`, and back to `true` to start it again.

If you've configured your `BrowserSystem` to enable the console plugin, you should see and be able to use your browser in the [_browsers_](/browsers) page (accessible under _>Networking>Browsers_).

## Uninstalling

Deleting the `BrowserSystem` removes the operator's components **and deletes every `Browser` in the cluster**. Delete or back up the `Browser` resources you want to keep before deleting the `BrowserSystem` or uninstalling the operator.
