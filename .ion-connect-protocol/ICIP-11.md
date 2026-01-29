ICIP-11
======

Relay Information Document
-----

`draft` `optional`

This ICIP extends [NIP-11](https://github.com/nostr-protocol/nips/blob/master/11.md) with additional fields or with modifications/extensions to the existing [NIP-11](https://github.com/nostr-protocol/nips/blob/master/11.md) fields.

### FCM client configs

As described in [ICIP-8000](ICIP-8000.md#fcm-client-configuration)

### System Metrics

A new `system_metrics` json field is added with the following fields:
1. used_file_storage
2. used_database_storage
3. used_total_storage
4. used_memory
5. used_cpu
6. used_bandwidth

###### Example
```json
{
  "name": <string identifying relay>,
  "description": <string with detailed information>,
  "banner": <a link to an image (e.g. in .jpg, or .png format)>,
  "icon": <a link to an icon (e.g. in .jpg, or .png format>,
  "pubkey": <administrative contact pubkey>,
  "contact": <administrative alternate contact>,
  "supported_nips": <a list of NIP numbers supported by the relay>,
  "software": <string identifying relay software URL>,
  "version": <string version identifier>
  "privacy_policy": <a link to a text file describing the relay's privacy policy>,
  "terms_of_service": <a link to a text file describing the relay's term of service>,
  "system_metrics": {
    "used_file_storage": <integer representing the storage in bytes>,
    "used_database_storage": <integer representing the storage in bytes>,
    "used_total_storage": <integer representing the storage in bytes>,
    "used_memory": <integer representing the amount in bytes>,
    "used_cpu": <integer representing the percentage>,
    "used_bandwidth": <integer representing the amount in bytes per second>
  }
}
```

### System status

A new `system_status` json field is added which returns a map of statuses for all the functionality the service/relay provides. 

The functionalities can be one of the following:
1. publishing_events
2. subscribing_for_events
3. dvm
4. uploading_files
5. reading_files
6. sending_push_notifications

The statuses can be one of the following:
1. UP
2. DOWN
3. MAINTENANCE

###### Example
```json
{
  "name": <string identifying relay>,
  "description": <string with detailed information>,
  "banner": <a link to an image (e.g. in .jpg, or .png format)>,
  "icon": <a link to an icon (e.g. in .jpg, or .png format>,
  "pubkey": <administrative contact pubkey>,
  "contact": <administrative alternate contact>,
  "supported_nips": <a list of NIP numbers supported by the relay>,
  "software": <string identifying relay software URL>,
  "version": <string version identifier>
  "privacy_policy": <a link to a text file describing the relay's privacy policy>,
  "terms_of_service": <a link to a text file describing the relay's term of service>,
  "system_metrics": {
    "used_file_storage": <integer representing the storage in bytes>,
    "used_database_storage": <integer representing the storage in bytes>,
    "used_total_storage": <integer representing the storage in bytes>,
    "used_memory": <integer representing the amount in bytes>,
    "used_cpu": <integer representing the percentage>,
    "used_bandwidth": <integer representing the amount in bytes per second>
  },
  "system_status": {
    "publishing_events": "UP",
    "uploading_files": "DOWN",
    "sending_push_notifications": "MAINTENANCE"
  }
}
```