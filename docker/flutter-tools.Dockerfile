FROM halaqaty-flutter-ci:local

USER root
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && npm install --global firebase-tools \
    && rm -rf /var/lib/apt/lists/* /root/.npm

ENV PATH="/root/.pub-cache/bin:${PATH}"
RUN dart pub global activate flutterfire_cli

WORKDIR /workspace/mobile
