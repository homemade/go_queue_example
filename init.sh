#!/usr/bin/env bash

# Basic System Requirements (make, gcc, curl, git, zip, unzip, python, ruby, postgres client)
sudo apt-get update
sudo apt-get install -y build-essential curl git zip unzip python2.7 ruby-full postgresql-client

# Go 1.6.2
curl -sO https://storage.googleapis.com/golang/go1.6.2.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.6.2.linux-amd64.tar.gz
rm go1.6.2.linux-amd64.tar.gz
sudo mkdir -p /mnt/GoWork/pkg
sudo mkdir -p /mnt/GoWork/bin

# Heroku tool belt
wget -O- https://toolbelt.heroku.com/install-ubuntu.sh | sh


# Setup user profile, generate ssh keys, add known hosts
if [ "$USER" == "root" ]; then
  # Swap user to `vagrant` when provisioning the local dev. env.
  echo "Setting up profile for user vagrant"
  su vagrant -c "printf 'export GOPATH=%s\n' /mnt/GoWork >> ~/.bashrc"
  su vagrant -c "echo 'PATH=$PATH:/usr/local/go/bin:/mnt/GoWork/bin' >> ~/.bashrc"
	sudo chown vagrant:vagrant /mnt/GoWork/src
	sudo chown vagrant:vagrant /mnt/GoWork/pkg
	sudo chown vagrant:vagrant /mnt/GoWork/bin
  sudo chown vagrant:vagrant /mnt/GoWork/src/**
  su vagrant -c "cat /dev/zero | ssh-keygen -q -N ''"
  su vagrant -c "ssh-keyscan bitbucket.org >> ~/.ssh/known_hosts 2>/dev/null"
  su vagrant -c "ssh-keyscan github.com >> ~/.ssh/known_hosts 2>/dev/null"
else
  # For other envs stick with current user
  echo "Setting up profile for user "$USER
  printf 'export GOPATH=%s\n' /mnt/GoWork >> ~/.bashrc
  echo 'PATH=$PATH:/usr/local/go/bin:/mnt/GoWork/bin' >> ~/.bashrc
  sudo chown $USER:$USER /mnt/GoWork/src
  sudo chown $USER:$USER /mnt/GoWork/pkg
  sudo chown $USER:$USER /mnt/GoWork/bin
  sudo chown $USER:$USER /mnt/GoWork/src/**
  cat /dev/zero | ssh-keygen -q -N ''
  ssh-keyscan bitbucket.org >> ~/.ssh/known_hosts 2>/dev/null
  ssh-keyscan github.com >> ~/.ssh/known_hosts 2>/dev/null
fi
