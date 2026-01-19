FROM ruby:3.3.9

WORKDIR /usr/src/app

RUN adduser --disabled-password --gecos --quiet pyroscope
USER pyroscope

COPY --from=pyroscope/pyroscope:latest /usr/bin/pyroscope /usr/bin/pyroscope
COPY --chown=pyroscope main.rb ./main.rb
COPY --chown=pyroscope Gemfile ./Gemfile
COPY --chown=pyroscope Gemfile.lock ./Gemfile.lock

ENV PYROSCOPE_APPLICATION_NAME=simple.ruby.app
ENV PYROSCOPE_SERVER_ADDRESS=http://pyroscope:4040/
ENV PYROSCOPE_LOG_LEVEL=debug

RUN bundle install

CMD ["ruby", "main.rb"]
