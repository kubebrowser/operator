## About

This operator integrates a full web-browsing capability directly into your cluster.
It introduces a `Browser` custom resource that deploys a Chromium browser inside a pod.
A built-in console plugin provides a page with VNC access to the browser, letting you browse the web directly from within your cluster environment.
A sidecar REST API allows programmatic remote control of the browser.
Without this operator, testing a service typically required creating a `Route`, exec’ing into another pod and running `curl`, or setting up a port-forward. Now you can simply open a browser from inside the cluster.

## Features

- Deploys Chromium browsers inside pods
- Provides a REST API for remote browser control
- Adds a **Networking -> Browsers** page to the web console for web browsing

## Purpose

The Browser Operator makes it easy to:

- Test services from within the cluster
- Validate network connectivity across namespaces
- Verify access to external services
- Debug routing, DNS, and access issues directly from the cluster’s perspective

## Installing

After installing the the operator, create a `BrowserSystem` instance. Configure the fields in `.spec` needed, otherwise leave it blank for defaults.

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

```
apiVersion: core.kubebrowser.io/v1alpha1
kind: Browser
metadata:
  name: mybrowser
spec:
  started: false
```

If you've configured your `BrowserSystem` to enable the console plugin, you should see and be able to use your browser in the [_browsers_](/browsers) page (accessible under _>Networking>Browsers_).
