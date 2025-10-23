FROM ubuntu:24.04

# Umgebungsvariablen setzen
ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=Europe/Berlin

# System-Updates und grundlegende Tools installieren
RUN apt-get update && apt-get install -y \
    curl \
    wget \
    git \
    unzip \
    software-properties-common \
    apt-transport-https \
    ca-certificates \
    gnupg \
    lsb-release \
    vim \
    nano \
    tree \
    htop \
    sudo \
    make \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Zeitzone konfigurieren
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone

# Erstelle einen non-root Benutzer
ARG USERNAME=vscode
ARG USER_UID=1001
ARG USER_GID=$USER_UID
RUN groupadd --gid $USER_GID $USERNAME \
    && useradd --uid $USER_UID --gid $USER_GID -m $USERNAME \
    && echo $USERNAME ALL=\(root\) NOPASSWD:ALL > /etc/sudoers.d/$USERNAME \
    && chmod 0440 /etc/sudoers.d/$USERNAME

# Shared bin Verzeichnis erstellen und Rechte setzen
RUN mkdir -p /shared/bin && chown -R $USERNAME:$USERNAME /shared/bin

# Arbeitsverzeichnis setzen
WORKDIR /workspace/plexams.go

# Benutzer wechseln
USER $USERNAME

# # Delve Debugger installieren
# RUN /usr/local/go/bin/go install github.com/go-delve/delve/cmd/dlv@latest
# ENV PATH="/home/vscode/go/bin:$PATH"

# Set GOBIN to shared volume
ENV GOBIN=/shared/bin
ENV PATH="${PATH}:/shared/bin"

# Standard-Befehl
CMD ["/bin/bash"]
