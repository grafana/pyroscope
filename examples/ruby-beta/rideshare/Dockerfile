FROM ruby:3

WORKDIR /opt/app

USER root

RUN curl https://sh.rustup.rs -sSf | sh -s -- --default-toolchain stable -y
ENV PATH=$PATH:/root/.cargo/bin

RUN gem install pyroscope_beta

COPY Gemfile ./Gemfile
COPY Gemfile.lock ./Gemfile.lock
# RUN bundle config set --local deployment true
RUN bundle install

COPY lib ./lib

CMD [ "ruby", "-v", "lib/server.rb" ]
