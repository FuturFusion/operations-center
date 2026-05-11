#!/bin/bash

incus project switch default
incus project delete tutorial-incusos-cluster --force
operations-center remote remove tutorial-operations-center
incus remote switch local
incus remote remove tutorial-incusos-cluster
rm -f ~/Downloads/IncusOS_OperationsCenter.iso
rm -f ~/Downloads/IncusOS.iso
