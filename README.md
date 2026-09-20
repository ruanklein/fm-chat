# FM Chat

FM Chat is a small macOS desktop chat that demonstrates [fmgo](https://github.com/ruanklein/fmgo), the Go interface for Apple's Foundation Models CLI.

It uses the on-device `system` model, streams responses, persists conversations locally, and keeps selected images inside the app's Application Support directory. No API key, account, or network service is involved.

## Requirements

- macOS 27 or later on Apple Silicon
- Apple Intelligence enabled
- Foundation Models CLI terms accepted
- The native `/usr/bin/fm` command available

## Development

```sh
wails dev
```

Build the signed local application with:

```sh
wails build
```

## Attachments

The current `fm` CLI accepts image paths. FM Chat supports PNG, JPEG, HEIC, GIF, and WebP images. Documents are intentionally not accepted because they are not an attachment type exposed by `fmgo` or the native CLI.
