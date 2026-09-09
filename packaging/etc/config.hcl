# Terraform HTTP Backend configuration file
#
# This file defines connection parameters used by the terrafrom-backend
# A simple HTTP backend for Terraform, using the file system for tfstate storage, written in Go.
#
# Configuration priority (highest to lowest):
#   1. CLI flags
#   2. Environment variables
#   3. HCL configuration file
#
# Default config location:
#   /etc/terraform-backend/config.hcl
#
# Environment variables supported:
#   TF_HTTP_ADDR
#   TF_HTTP_PATH
#   TF_HTTP_CONFIG
#   TF_HTTP_DEBUG
#

# The address to which HTTP server will bind.
# address = "127.0.0.1:3001"

# The path to Terraform state files storage.
# path = /var/lib/terraform-backend/state

# Enables debug mode.
# debug = false
