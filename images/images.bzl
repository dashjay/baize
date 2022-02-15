load("//build:def.bzl", "bundle_image")

_components = [
    "baize-executor",
    "baize-remote-cache",
    "baize-server",
]

images = bundle_image({
    "brain/%s" % comp: "//images/%s:image" % comp
    for comp in _components
})
