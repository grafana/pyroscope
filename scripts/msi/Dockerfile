FROM harbottle/wix AS msi-buileder

COPY pyroscope.wsx pyroscope.wsx

# TODO: FROM pyroscope-builer
COPY pyroscope.exe pyroscope.exe

RUN candle -arch x64 -ext WixUtilExtension \
    -dPyroscopeSourceExecutable=pyroscope.exe \
    pyroscope.wsx

RUN light -sval pyroscope.wixobj

FROM scratch AS msi-exporter
COPY --from=msi-buileder /mnt/workspace/pyroscope.msi /
