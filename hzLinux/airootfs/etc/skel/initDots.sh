#!/usr/bin/env bash

mkdir -p $HOME/Git/Github
git clone https://github.com/HaoZeke/Dotfiles $HOME/Git/Github/Dotfiles
# cd $HOME/Git/Github/Dotfiles
# dotgit restore hzArchiso
sh -c "$(curl -fsLS chezmoi.io/get)" -- init --apply HaoZeke --branch chezmoi
chezmoi init
