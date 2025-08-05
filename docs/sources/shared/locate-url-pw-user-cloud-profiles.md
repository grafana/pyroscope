---
headless: true
description: Shared file for available profile types.
---

[//]: # 'This file where to locate the username, password, and URL in Cloud Profiles.'
[//]: # 'This shared file is included in these locations:'
[//]: # '/website/docs/grafana-cloud/monitor-applications/profiles/send-profile-data.md'
[//]: #
[//]: #
[//]: # 'If you make changes to this file, verify that the meaning and content are not changed in any place where the file is included.'
[//]: # 'Any links should be fully qualified and not relative: /docs/grafana/ instead of ../grafana/.'

<!-- Locate your stack's URL, user, and password -->

When you configure Alloy or your SDK, you need to provide the URL, user, and password for your Grafana Cloud stack.
This information is located in the **Pyroscope** section of your Grafana Cloud stack.

1. Navigate to your Grafana Cloud stack.
1. Select **Details** next to your stack.
1. Locate the **Pyroscope** section and select **Details**.
1. Copy the **URL**, **User**, and **Password** values in the **Configure the client and data source using Grafana credentials** section.
   ![Locate the SDK or Grafana Alloy configuration values](/media/docs/pyroscope/cloud-profiles-url-user-password.png)
1. Use these values to complete the configuration.

As an alternative, you can also create a Cloud Access Policy and generate a token to use instead of the user and password.
For more information, refer to [Create a Cloud Access Policy](https://grafana.com/docs/grafana-cloud/security-and-account-management/authentication-and-permissions/access-policies/create-access-policies/).