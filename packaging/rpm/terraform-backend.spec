Name:           terraform-backend
Version:        VERSION
Release:        1%{?dist}
Summary:        Terraform HTTP Backend
License:        MIT
URL:            https://github.com/kirill-shtrykov/terraform-http-backend
Group:          Utilities
BuildArch:      x86_64
Packager:       Kirill Shtrykov <kirill@shtrykov.com>

%description
A simple HTTP backend for Terraform,
using the file system for tfstate storage, written in Go.

%install
mkdir -p %{buildroot}/usr/bin
mkdir -p %{buildroot}/etc/terraform-backend
mkdir -p %{buildroot}/usr/lib/systemd/system
install -m 0755 %{_sourcedir}/terraform-backend %{buildroot}/usr/bin/
install -m 0644 %{_sourcedir}/config.hcl %{buildroot}/etc/terraform-backend/config.hcl
install -m 0644 %{_sourcedir}/terraform-backend.service %{buildroot}/usr/lib/systemd/system/terraform-backend.service

%pre
# Create system user and group if they do not exist.
getent group terraform >/dev/null 2>&1 || groupadd --system terraform
getent passwd terraform >/dev/null 2>&1 || \
    useradd --system --no-create-home --gid terraform terraform

%post
systemctl daemon-reload >/dev/null 2>&1 || :
%systemd_post terraform-backend.service

# Create state directory with correct ownership.
mkdir -p /var/lib/terraform-backend/state
chown terraform:terraform /var/lib/terraform-backend/state
chmod 0750 /var/lib/terraform-backend/state

%preun
%systemd_preun terraform-backend.service

%postun
systemctl daemon-reload >/dev/null 2>&1 || :
%systemd_postun_with_restart terraform-backend.service

%files
/usr/bin/terraform-backend
/etc/terraform-backend/config.hcl
/usr/lib/systemd/system/terraform-backend.service

%changelog
* Thu Mar 12 2026 Kirill Shtrykov<kirill@shtrykov.com> - VERSION-1
- Initial package
