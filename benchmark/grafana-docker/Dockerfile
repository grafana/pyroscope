FROM kamon/grafana_graphite:latest

RUN     rm -rf /src/grafana &&\
        rm -rf /opt/grafana &&\
        mkdir /src/grafana                                                                                           &&\
        mkdir /opt/grafana                                                                                           &&\
        curl https://dl.grafana.com/oss/release/grafana-7.5.7.linux-amd64.tar.gz  \
             -o /src/grafana.tar.gz                                                                                  &&\
        tar -xzf /src/grafana.tar.gz -C /opt/grafana --strip-components=1                                            &&\
        rm /src/grafana.tar.gz

COPY ./custom.ini /opt/grafana/conf/custom.ini
