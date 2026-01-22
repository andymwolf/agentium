packer {
  required_plugins {
    googlecompute = {
      source  = "github.com/hashicorp/googlecompute"
      version = ">= 1.1.0"
    }
  }
}

source "googlecompute" "agentium" {
  project_id          = var.project_id
  zone                = var.zone
  machine_type        = var.machine_type
  source_image_family = "ubuntu-2204-lts"
  source_image_project_id = ["ubuntu-os-cloud"]
  ssh_username        = "packer"
  disk_size           = var.disk_size_gb
  disk_type           = "pd-ssd"
  image_name          = "agentium-${var.image_version}-{{timestamp}}"
  image_family        = "agentium"
  image_description   = "Agentium pre-baked image with Docker, Node.js, Claude Code, and other tools"
  image_labels = {
    agentium = "true"
    version  = var.image_version
  }
}

build {
  sources = ["source.googlecompute.agentium"]

  # Upload provisioning script
  provisioner "file" {
    source      = "${path.root}/scripts/provision.sh"
    destination = "/tmp/provision.sh"
  }

  # Run provisioning script
  provisioner "shell" {
    inline = [
      "chmod +x /tmp/provision.sh",
      "sudo /tmp/provision.sh",
    ]
  }

  # Clean up for image creation
  provisioner "shell" {
    inline = [
      "sudo apt-get clean",
      "sudo rm -rf /var/lib/apt/lists/*",
      "sudo rm -rf /tmp/*",
      "sudo rm -rf /var/tmp/*",
      "sudo truncate -s 0 /var/log/*.log",
      "sudo truncate -s 0 /var/log/**/*.log 2>/dev/null || true",
      "sudo sync",
    ]
  }
}
