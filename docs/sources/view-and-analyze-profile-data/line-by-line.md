---
description: Integrate your source code on GitHub with Pyroscope profiling data.
keywords:
  - GitHub
  - continuous profiling
  - flame graphs
menuTitle: Show usage line by line
title: Integrate your source code on GitHub with Pyroscope profiling data.
weight: 550
---

# Integrate your source code on GitHub with Pyroscope profiling data.

The Grafana Pyroscope GitHub integration offers seamless integration between your code repositories and Grafana.
Using this app, you can map your code directly within Grafana and visualize resource performance line by line.
With these powerful capabilities, you can gain deep insights into your code's execution and identify performance bottlenecks.

Every profile type works for the integration for code written in Go.
For information on profile types and the profiles available with Go, refer to [Profiling types and their uses](../../introduction/profiling-types/).

![Example of a flame graph with the function details populated](/media/docs/grafana-cloud/profiles/screenshot-profiles-github-integration.png)

## How it works

The Pyroscope GitHub integration uses labels configured in the application being profiled to associate profiles with source code.
The integration is only available for Go applications.
The Go profiler can map symbolic information, such as function names and line numbers, to source code.

The Pyroscope GitHub integration uses three labels, `service_repository`, `service_git_ref`, and `service_root_path`, to add commit information, repository link, and an enhanced source code preview to the **Function Details** screen.

{{< admonition type="note" >}}
The source code mapping is only available to people who have access to the source code in GitHub.
{{< /admonition >}}

## Before you begin

To use the Pyroscope integration for GitHub, you need an application that emits profiling data, a GitHub account, and a Grafana instance with a Grafana Pyroscope backend.

### Application with profiling data requirements

{{< admonition type="warning" >}}
  - Applications in other languages aren't supported
  - eBPF profiled Go workloads aren't supported
{{< /admonition >}}

- A Go application which is profiled by Grafana Alloy's `pyroscope.scrape` or using the [Go Push SDK](../../configure-client/language-sdks/go_push/).


Your Go application provides the following labels (tags):

- `service_git_ref` points to the Git commit or [reference](https://docs.github.com/en/rest/git/refs?apiVersion=2022-11-28#about-git-references) from which the binary was built
- `service_repository` is the GitHub repository that hosts the source code
- `service_root_path` (Optional) is the path where the code lives inside the repository

To activate this integration, you need to add at least the two mandatory labels when sending profiles:
`service_repository` and `service_git_ref`.
They should respectively be set to the full repository GitHub URL and the current
[`git ref`](https://docs.github.com/en/rest/git/refs?apiVersion=2022-11-28#about-git-references).

```go
pyroscope.Start(pyroscope.Config{
    Tags: map[string]string{
      "service_git_ref":    "<GIT_REF>",
      "service_repository": "https://github.com/<YOUR_ORG>/<YOUR_REPOSITORY>",
      "service_root_path": "<PATH_TO_SERVICE_ROOT>", // optional
    },
    // Other configuration
  })
```

### GitHub requirements

- A GitHub account
- Source code hosted on GitHub

{{< admonition type="note" >}}
Data from your GitHub repository may be limited if your GitHub organization or repository restricts third-party applications.
For example, the organization may need to add this app to an allowlist to access organizational resources.
Contact your organization administrator for assistance.
Refer to [Requesting a GitHub App from your organization owner](https://docs.github.com/en/apps/using-github-apps/requesting-a-github-app-from-your-organization-owner).
{{< /admonition >}}

### Ensure the GitHub integration is configured in Grafana Pyroscope

Refer to [Configure GitHub integration](../../configure-server/configuring-github-integration/) on the steps required.

### Ensure the Grafana Pyroscope data source is configured correctly

In order to make use of the GitHub integration, the Pyroscope data source needs to be configured to pass a particular cookie through.

To configure cookie passthrough in Grafana:

1. Navigate to **Configuration** > **Data sources** in Grafana.
1. Select your Pyroscope data source.
1. Under **Additional settings** > **Advanced HTTP settings** , locate **Allowed cookies**.
1. Add `pyroscope_git_session` to the list of cookies to forward.
1. Click **Save & test** to apply the changes.


![Additional data source settings for Pyroscope GitHub integration](/media/docs/pyroscope/pyroscope-data-source-additional-settings.png)

{{< admonition type="note" >}}
Cookie forwarding must be enabled for the GitHub integration to work properly. Without it, you won't be able to connect to GitHub repositories from within Grafana Profiles Drilldown.
{{< /admonition >}}


## Authorize access to GitHub

You can authorize with GitHub using the **Connect to GitHub** in the **Function Details** panel.

1. From within **Single view** with a configured Pyroscope app plugin.
1. Select **Pyroscope service**. For this example, select `cpu_profile`.
1. Click in the flame graph on a function you want to explore. Select **Function details**.
1. On **Function Details**, locate the **Repository** field and select **Connect to \<GITHUB REPOSITORY\>**, where `<GITHUB REPOSITORY>` is replaced with the repository name where the files reside. In this case, it’s connecting to the `grafana/pyroscope` repository.
1. If prompted, log in to GitHub.
1. After Grafana connects to your GitHub account, review the permissions and select **Authorize Grafana Pyroscope**.

{{< admonition type="note" >}}
Organization owners may disallow third-party apps for the entire organization or specific organization resources, like repositories.
If this is the case, you won't be able authorize the Grafana Pyroscope GitHub integration to view source code or commit information for the protected resources.
{{< /admonition >}}

### Modify or remove the Pyroscope GitHub integration from your GitHub account

The Pyroscope GitHub integration uses a GitHub app called "Grafana Pyroscope" to connect to GitHub.
This application authorizes Grafana Cloud to access source code and commit information.

After authorizing the app, your GitHub account, **GitHub** > **Settings** > **Applications** lists the Grafana Pyroscope app.

You can change the repositories the Pyroscope GitHub integration can access on the **Applications** page.

You can use also remove the app's permissions by selecting **Revoke**.
Revoking the permissions disables the integration in your Grafana Cloud account.

For more information about GitHub applications:

- [Using GitHub apps](https://docs.github.com/en/apps/using-github-apps/about-using-github-apps)
- [Authorizing GitHub apps](https://docs.github.com/en/apps/using-github-apps/authorizing-github-apps)
- [Differences between installing and authorizing apps](https://docs.github.com/en/apps/using-github-apps/installing-a-github-app-from-a-third-party#difference-between-installation-and-authorization)

## How your GitHub code shows up in profile data queries

After authorizing the Pyroscope Grafana integration, you see more details in the **Function Details** from flame graphs in Profiles Drilldown.

1. Open a browser to your Pyroscope instance.
1. Sign in to your account, if prompted.
1. After the Grafana instance loads, select **Drilldown**.
1. Next, select **Profiles** > **Single view** from the left-side menu.
1. Optional: Select a **Service** and **Profile**.
1. Click in the flame graph and select **Function details** from the pop-up menu.

### Function Details

The Function Details section provide information about the function you selected from the flame graph.

{{< figure max-width="80%" class="center" caption-align="center" src="/media/docs/grafana-cloud/profiles/screenshot-profiles-github-funct-details-v3.png" caption="Function Details panel from a connected Pyroscope GitHub integration." >}}

The table explains the main fields in the table.
The values for some of the fields, such as Self and Total, change depending whether a profile uses time or memory amount.
Refer to [Understand Self versus Total metrics in profiling with Pyroscope](https://grafana.com/docs/pyroscope/latest/view-and-analyze-profile-data/self-vs-total/) for more information.

| Field                      | Meaning                                                                                                                                                                                          | Notes                                                                        |
| -------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------- |
| Function name              | The name of the selected function                                                                                                                                                                |                                                                              |
| Start time                 | The line where the function definition starts                                                                                                                                                    |                                                                              |
| File                       | Path where the function is defined                                                                                                                                                               | You can use the clipboard icon to copy the path.                             |
| Repository                 | The repository configured for the selected service                                                                                                                                               |                                                                              |
| Commit                     | The version of the application (commit) where the function is defined. Use the drop-down menu to target a specific commit.                                                                       | Click the Commit ID to view the commit in the repository.                    |
| Breakdown per line (table) | Provides the function location in the code and self and total values.                                                                                                                            |                                                                              |
| Self                       | ‘Self’ refers to the resource usage (CPU time, memory allocation, etc.) directly attributed to a specific function or a code segment, excluding the resources used by its sub-functions or calls | This value can be a time or memory amount depending on the profile selected. |
| Total                      | ‘Total’ encompasses the combined resource usage of a function along with all the functions it calls.                                                                                             | This value can be a time or memory amount depending on the profile selected. |
