# Secure Boot Certificates

## Microsoft

Download official Microsoft Secure Boot Certificates linked in the
[Windows Secure Boot Key Creation and Management Guidance](https://learn.microsoft.com/en-us/windows-hardware/manufacture/desktop/windows-secure-boot-key-creation-and-management-guidance?view=windows-11).

* [Microsoft Corporation UEFI CA 2011](https://go.microsoft.com/fwlink/p/?linkid=321194) - SHA1 fingerprint: `46DEF63B5CE61CF8BA0DE2E6639C1019D0ED14F3`, SHA256 fingerprint: `48e99b991f57fc52f76149599bff0a58c47154229b9f8d603ac40d3500248507`
  Used to sign ROM shipped before ~2024
* [Microsoft UEFI CA 2023](https://go.microsoft.com/fwlink/?linkid=2239872) - SHA1 fingerprint: `B5EEB4A6706048073F0ED296E7F580A790B59EAA`, SHA256 fingerprint: `f6124e34125bee3fe6d79a574eaa7b91c0e7bd9d929c1a321178efd611dad901`
  Required to boot e.g. distro shim, not needed in our case.
* [Microsoft Option ROM UEFI CA 2023](https://go.microsoft.com/fwlink/?linkid=2284009) - SHA1 fingerprint: `3FB39E2B8BD183BF9E4594E72183CA60AFCD4277`, SHA256 fingerprint: `e5be3e64c6e66a281457ecdece0d6d0787577aad2a3a0144262c10c14ba8d8f1`
  Used to sign ROM shipped after ~2024
* [Windows UEFI CA 2023](https://go.microsoft.com/fwlink/?linkid=2239776) - SHA1 fingerprint: `45A0FA32604773C82433C3B7D59E7466B3AC0C67`, SHA256 fingerprint: `076f1fea90ac29155ebf77c17682f75f1fdd1be196da302dc8461e350a9ae330`
  Required to boot Windows, not needed in our case
