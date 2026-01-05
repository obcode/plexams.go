FROM ubuntu:24.04

# Umgebungsvariablen setzen
ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=Europe/Berlin
ENV LANG=C.UTF-8
ENV LC_ALL=C.UTF-8

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
    sudo \
    zsh \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Zeitzone konfigurieren
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone

ARG USERNAME=ubuntu
ARG USER_UID=1000
RUN echo $USERNAME ALL=\(root\) NOPASSWD:ALL > /etc/sudoers.d/$USERNAME \
    && chmod 0440 /etc/sudoers.d/$USERNAME

# Shared bin Verzeichnis erstellen und Rechte setzen
RUN mkdir -p /shared/bin && chown -R $USERNAME:$USERNAME /shared/bin

# Arbeitsverzeichnis setzen
WORKDIR /workspace/plexams.go

# starship installieren (als root, damit keine sudo-Anforderung)
RUN curl -sS https://starship.rs/install.sh | sh -s -- -y

# Benutzer wechseln
USER $USERNAME

# oh-my-zsh installieren
RUN sh -c "$(curl -fsSL https://raw.githubusercontent.com/ohmyzsh/ohmyzsh/master/tools/install.sh)" "" --unattended

# oh-my-zsh Plugins installieren
RUN git clone https://github.com/zsh-users/zsh-autosuggestions ${ZSH_CUSTOM:-~/.oh-my-zsh/custom}/plugins/zsh-autosuggestions && \
    git clone https://github.com/zsh-users/zsh-syntax-highlighting.git ${ZSH_CUSTOM:-~/.oh-my-zsh/custom}/plugins/zsh-syntax-highlighting

# Starship-Konfiguration anlegen
RUN mkdir -p /home/${USERNAME}/.config
COPY starship.toml /home/${USERNAME}/.config/starship.toml

# starship in .zshrc einbinden
RUN echo 'eval "$(starship init zsh)"' >> ~/.zshrc

# zsh als Standard-Shell setzen
RUN sudo chsh -s $(which zsh) $USERNAME

# .zshrc mit oh-my-zsh Plugins konfigurieren
RUN sed -i 's/plugins=(git)/plugins=(git golang docker docker-compose sudo zsh-autosuggestions zsh-syntax-highlighting)/g' ~/.zshrc

# # Delve Debugger installieren
# RUN /usr/local/go/bin/go install github.com/go-delve/delve/cmd/dlv@latest
# ENV PATH="/home/vscode/go/bin:$PATH"

# Set GOBIN to shared volume
ENV GOBIN=/shared/bin
ENV PATH="${PATH}:/shared/bin"

# Standard-Befehl
CMD ["/bin/bash"]
