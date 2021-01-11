FROM ruby:2.7.1

WORKDIR /usr/src/app

COPY --from=pyroscope/pyroscope:latest /usr/bin/pyroscope /usr/bin/pyroscope
COPY main.rb ./main.rb

ENV PYROSCOPE_APPLICATION_NAME=simple.ruby.app
ENV PYROSCOPE_SERVER_ADDRESS=http://pyroscope:4040/
CMD ["pyroscope", "exec", "ruby", "main.rb"]
