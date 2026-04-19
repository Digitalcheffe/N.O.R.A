# Tautulli Webhook Configuration for NORA

For each trigger below, create a separate Notification Agent in Tautulli:
**Settings → Notification Agents → Add → Webhook**

- Webhook URL: `{base_url}/ingest/{token}`
- Method: `POST`
- Content Type: `application/json`

Paste the JSON body into the **JSON Body** field for each trigger.

---

## Playback Start

**Trigger:** Playback Start

```json
{
  "action": "playback.start",
  "user": "{user}",
  "title": "{title}",
  "media_type": "{media_type}",
  "show_name": "{show_name}",
  "season_num": "{season_num00}",
  "episode_num": "{episode_num00}",
  "year": "{year}",
  "player": "{player}",
  "platform": "{platform}",
  "stream_location": "{stream_location}",
  "transcode_decision": "{transcode_decision}",
  "library_name": "{library_name}",
  "server_name": "{server_name}"
}
```

---

## Watched

**Trigger:** Watched

```json
{
  "action": "watched",
  "user": "{user}",
  "title": "{title}",
  "media_type": "{media_type}",
  "show_name": "{show_name}",
  "season_num": "{season_num00}",
  "episode_num": "{episode_num00}",
  "year": "{year}",
  "player": "{player}",
  "platform": "{platform}",
  "stream_location": "{stream_location}",
  "library_name": "{library_name}",
  "server_name": "{server_name}"
}
```

---

## Recently Added

**Trigger:** Recently Added

```json
{
  "action": "recently_added",
  "title": "{title}",
  "media_type": "{media_type}",
  "show_name": "{show_name}",
  "season_num": "{season_num00}",
  "episode_num": "{episode_num00}",
  "year": "{year}",
  "library_name": "{library_name}",
  "server_name": "{server_name}"
}
```

---

## User New Device

**Trigger:** User New Device

```json
{
  "action": "user_new_device",
  "user": "{user}",
  "player": "{player}",
  "platform": "{platform}",
  "ip_address": "{ip_address}",
  "server_name": "{server_name}"
}
```

---

## Plex Server Down

**Trigger:** Plex Server Down

```json
{
  "action": "plex_server_down",
  "server_name": "{server_name}"
}
```

---

## Plex Server Back Up

**Trigger:** Plex Server Back Up

```json
{
  "action": "plex_server_up",
  "server_name": "{server_name}"
}
```

---

## Plex Remote Access Down

**Trigger:** Plex Remote Access Down

```json
{
  "action": "plex_remote_down",
  "server_name": "{server_name}",
  "remote_access_reason": "{remote_access_reason}"
}
```

---

## Plex Remote Access Back Up

**Trigger:** Plex Remote Access Back Up

```json
{
  "action": "plex_remote_up",
  "server_name": "{server_name}"
}
```

---

## Plex Update Available

**Trigger:** Plex Update Available

```json
{
  "action": "plex_update",
  "server_name": "{server_name}",
  "update_version": "{update_version}",
  "update_release_date": "{update_release_date}",
  "update_channel": "{update_channel}"
}
```

---

## Tautulli Update Available

**Trigger:** Tautulli Update Available

```json
{
  "action": "tautulli_update",
  "server_name": "{server_name}",
  "tautulli_update_version": "{tautulli_update_version}"
}
```

---

## Tautulli Database Corruption

**Trigger:** Tautulli Database Corruption

```json
{
  "action": "tautulli_db_corruption",
  "server_name": "{server_name}"
}
```

---

## Tautulli Plex Token Expired

**Trigger:** Tautulli Plex Token Expired

```json
{
  "action": "tautulli_token_expired",
  "server_name": "{server_name}"
}
```
