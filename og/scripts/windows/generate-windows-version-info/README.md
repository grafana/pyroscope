# Windows Version Info

This tool generates `syso` file which is required for windows build.
In particular, version info is used in MSI build.

`go` embeds .syso object files at build, therefore the file should
be generated in advance.

Refer for details: https://docs.microsoft.com/en-us/windows/win32/menurc/versioninfo-resource?redirectedfrom=MSDN
