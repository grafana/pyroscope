FROM harbottle/wix AS msi-buileder

COPY pyroscope.wsx pyroscope.wsx

# TODO: FROM pyroscope-builer
COPY pyroscope.exe pyroscope.exe

# TODO: inject versoin info into pyroscope.exe and get rid of ProductVersion.
RUN candle -arch x64 -ext WixUtilExtension \
    -dPyroscopeSourceExecutable=pyroscope.exe \
    -dProductVersion=0.0.0 \
    pyroscope.wsx

RUN light -sval pyroscope.wixobj

FROM scratch AS msi-exporter
COPY --from=msi-buileder /mnt/workspace/pyroscope.msi /
