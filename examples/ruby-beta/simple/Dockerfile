FROM ruby:3.0.1

WORKDIR /usr/src/app

USER root

RUN gem install pyroscope_beta -v 0.2.3

COPY main.rb ./main.rb

CMD ["ruby", "main.rb"]
