---
description: Learn how to configure the GitHub integration for Grafana Pyroscope.
menuTitle: Configure GitHub integration
title: Configure GitHub integration for Grafana Pyroscope
weight: 550
---

# Configuring GitHub Integration

This guide walks you through setting up the GitHub integration with minimal permissions for Grafana Pyroscope.

## Creating a GitHub App

1. Go to your GitHub account settings
2. Navigate to **Developer settings** > **GitHub Apps**
3. Click **New GitHub App**
4. Configure the app with the following settings:
    #### **Basic Information**
      - **GitHub App name**: Choose a name for your app (e.g., "my-pyroscope")
      - **Homepage URL**: This is a required field, you can use any URL. (e.g., `https://grafana.com/oss/pyroscope/`)
      - **Callback URL**: Set this to your Grafana installation URL with the GitHub callback path. (e.g., `https://grafana.your-domain.com/a/grafana-pyroscope-app/github/callback`)

    #### Permissions

      The GitHub App works without any extra permissions for public repositories. If you want to access private repositories, you need to add these permissions:

      - **Repository permissions**:
        - **Metadata**: Read-only access
        - **Contents**: Read-only access

    #### Where can this GitHub App be installed?
      - Select **Any account** if you want to allow installation on any GitHub account
      - Select **Only on this account** if you want to restrict installation to your account only
5. Click **Create GitHub App**
6. After creating the GitHub App, you should end up in the GitHub App settings, find the **Client ID** and take a note of it.
7. Now scroll down to the **Client secrets** section and click **Generate a new client secret**
8. **Important**: Copy the generated client secret immediately - you won't be able to see it again after closing the dialog

For anything not covered in this guide, you can refer to the GitHub docs: [Registering a GitHub App](https://docs.github.com/en/apps/creating-github-apps/registering-a-github-app/registering-a-github-app).


## Configuring Pyroscope

This section explains how to configure the GitHub integration in Grafana Pyroscope. The integration requires three environment variables to be set:

| Variable | Description | Required |
|----------|-------------|----------|
| `GITHUB_CLIENT_ID` | The Client ID of your GitHub App | Yes |
| `GITHUB_CLIENT_SECRET` | The Client Secret of your GitHub App | Yes |
| `GITHUB_SESSION_SECRET` | A random string used to encrypt the session | Yes |

### Using the Helm Chart

If you're using the official Helm chart, follow these steps to configure the GitHub integration:

1. Create a Kubernetes secret containing the required values, this will also generate a new random session secret:

    ```bash
    kubectl create secret generic pyroscope-github \
      "--from-literal=client_id=<The Client ID from the 6. step>" \
      "--from-literal=client_secret=<The Client secret from the 8. step>" \
      "--from-literal=session_secret=$(openssl rand -base64 48)"
    ```

2. Update your `values.yaml` to expose these secrets as environment variables:

    ```yaml
    pyroscope:
      extraEnvVars:
        GITHUB_CLIENT_ID:
          valueFrom:
            secretKeyRef:
              name: pyroscope-github
              key: client_id
        GITHUB_CLIENT_SECRET:
          valueFrom:
            secretKeyRef:
              name: pyroscope-github
              key: client_secret
        GITHUB_SESSION_SECRET:
          valueFrom:
            secretKeyRef:
              name: pyroscope-github
              key: session_secret
    ```

3. Apply the changes using helm upgrade:


### Other Deployment Methods

For other deployment methods, ensure the same environment variables are set in your deployment configuration.

## Verifying the Integration

The configuration of the GitHub integration is now completed. In order to verify everything works as expected follow the user guide: [Integrate your source code on GitHub with Pyroscope profiling data](../../view-and-analyze-profile-data/line-by-line/).
