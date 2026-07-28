ARG DOTNET_IMAGE=mcr.microsoft.com/dotnet/sdk:8.0.423-bookworm-slim

FROM ${DOTNET_IMAGE} AS build
ARG PACKAGE_VERSION=1.0.0
RUN apt-get update \
    && apt-get install --yes --no-install-recommends binutils clang zlib1g-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY sdk/csharp/WorldEngine.Runtime.Abstractions/ sdk/csharp/WorldEngine.Runtime.Abstractions/
COPY sdk/csharp/WorldEngine.Runtime.NativeAot/ sdk/csharp/WorldEngine.Runtime.NativeAot/
COPY sdk/csharp/testapps/CounterModule/ sdk/csharp/testapps/CounterModule/

RUN dotnet pack sdk/csharp/WorldEngine.Runtime.Abstractions/WorldEngine.Runtime.Abstractions.csproj \
      --configuration Release \
      --output /packages \
      -p:PackageVersion=${PACKAGE_VERSION} \
    && dotnet pack sdk/csharp/WorldEngine.Runtime.NativeAot/WorldEngine.Runtime.NativeAot.csproj \
      --configuration Release \
      --output /packages \
      -p:PackageVersion=${PACKAGE_VERSION}

RUN dotnet restore sdk/csharp/testapps/CounterModule/CounterModule.Packaged.csproj \
      --artifacts-path /tmp/counter-artifacts \
      --packages /tmp/counter-packages \
      --runtime linux-x64 \
      -p:WorldEngineRuntimeVersion=${PACKAGE_VERSION} \
      --source /packages \
      --source https://api.nuget.org/v3/index.json

RUN dotnet publish sdk/csharp/testapps/CounterModule/CounterModule.Packaged.csproj \
    --no-restore \
    --artifacts-path /tmp/counter-artifacts \
    --configuration Release \
    --runtime linux-x64 \
    -p:WorldEngineRuntimeVersion=${PACKAGE_VERSION} \
    --output /publish

RUN test -f /publish/WorldEngine.Runtime.CounterFixture.so
RUN for symbol in \
      cardinal_runtime_v1_get_contract \
      cardinal_runtime_v1_create \
      cardinal_runtime_v1_initialize \
      cardinal_runtime_v1_tick \
      cardinal_runtime_v1_query \
      cardinal_runtime_v1_snapshot \
      cardinal_runtime_v1_restore \
      cardinal_runtime_v1_last_error \
      cardinal_runtime_v1_destroy; \
    do \
      nm --dynamic --defined-only /publish/WorldEngine.Runtime.CounterFixture.so \
        | grep --extended-regexp --quiet " ${symbol}(@@[^ ]+)?$"; \
    done

FROM scratch AS artifact
COPY --from=build /publish/WorldEngine.Runtime.CounterFixture.so /libcounter_fixture.so
