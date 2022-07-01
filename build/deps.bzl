load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

GOPROXY = "https://goproxy.io"

def deps():
    http_archive(
        name = "io_k8s_repo_infra",
        strip_prefix = "repo-infra-0.2.3",
        sha256 = "23d93e6e6ef656661d36b2afd301d277692ded016abe558650b4c813c7c369cf",
        urls = [
            "https://github.com/kubernetes/repo-infra/archive/v0.2.3.tar.gz",
        ],
    )
    http_archive(
        name = "io_bazel_rules_go",
        sha256 = "03a99c32480fff238f229105d6bac3da8207c5a1e7101b5bed8e3d1fa3c05da0",
        strip_prefix = "github.com/bazelbuild/rules_go@v0.29.0",
        urls = ["{}/github.com/bazelbuild/rules_go/@v/v0.29.0.zip".format(GOPROXY)],
    )

    http_archive(
        name = "bazel_gazelle",
        sha256 = "de69a09dc70417580aabf20a28619bb3ef60d038470c7cf8442fafcf627c21cb",
        urls = [
            "https://mirror.bazel.build/github.com/bazelbuild/bazel-gazelle/releases/download/v0.24.0/bazel-gazelle-v0.24.0.tar.gz",
            "https://github.com/bazelbuild/bazel-gazelle/releases/download/v0.24.0/bazel-gazelle-v0.24.0.tar.gz",
        ],
    )

    http_archive(
        name = "bazel_skylib",
        urls = ["{}/github.com/bazelbuild/bazel-skylib/@v/v0.0.0-20191009164321-e59b620b392a.zip".format(GOPROXY)],
        sha256 = "5c0268703657f508891311f51e8abe61bcc5bf5322ff435202935c780338919e",
        strip_prefix = "github.com/bazelbuild/bazel-skylib@v0.0.0-20191009164321-e59b620b392a",
        type = "zip",
    )
    http_archive(
        name = "com_google_protobuf",
        sha256 = "4ded24230583913b5206a0a20d27b7d19b357bf466bd5cdac994ce3c1c8cbc84",
        strip_prefix = "github.com/protocolbuffers/protobuf@v3.14.0+incompatible",
        type = "zip",
        urls = [
            "{}/github.com/protocolbuffers/protobuf/@v/v3.14.0+incompatible.zip".format(GOPROXY),
        ],
    )
    http_archive(
        name = "rules_python",
        urls = ["https://github.com/bazelbuild/rules_python/archive/0.6.0.tar.gz"],
        strip_prefix = "rules_python-0.6.0",
        sha256 = "a30abdfc7126d497a7698c29c46ea9901c6392d6ed315171a6df5ce433aa4502",
    )
    http_archive(
        name = "zlib",
        urls = ["{}/github.com/madler/zlib/@v/v1.2.11.zip".format(GOPROXY)],
        sha256 = "9355229ce4879fe2cfb2ff6ca835076fef41f8c8f9df5f390caef17dcbc2b924",
        build_file_content = """
licenses(["notice"])  #  BSD/MIT-like license

filegroup(
    name = "srcs",
    srcs = glob(["**"]),
    visibility = ["//third_party:__pkg__"],
)

filegroup(
    name = "embedded_tools",
    srcs = glob(["*.c"]) + glob(["*.h"]) + ["BUILD"] + ["LICENSE.txt"],
    visibility = ["//visibility:public"],
)

cc_library(
    name = "zlib",
    srcs = glob(["*.c"]),
    hdrs = glob(["*.h"]),
    # Use -Dverbose=-1 to turn off zlib's trace logging. (#3280)
    copts = [
        "-w",
        "-Dverbose=-1",
    ],
    includes = ["."],
    visibility = ["//visibility:public"],
)""",
    )

    http_archive(
        name = "shellcheck_linux_amd64",
        strip_prefix = "shellcheck-v0.8.0",
        sha256 = "ab6ee1b178f014d1b86d1e24da20d1139656c8b0ed34d2867fbb834dad02bf0a",
        urls = [
            "https://github.com/koalaman/shellcheck/releases/download/v0.8.0/shellcheck-v0.8.0.linux.x86_64.tar.xz",
        ],
        build_file = "//third_party:export.BUILD",
    )

    http_archive(
        name = "shellcheck_darwin_amd64",
        strip_prefix = "shellcheck-v0.8.0",
        sha256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
        urls = [
            "https://github.com/koalaman/shellcheck/releases/download/v0.8.0/shellcheck-v0.8.0.darwin.x86_64.tar.xz",
        ],
        build_file = "//third_party:export.BUILD",
    )
    http_archive(
        name = "golangci-lint_darwin_amd64",
        sha256 = "6a7c31abca3f51714e5ea1f0aae5dc78d72e7d57d07f02b1a1778219d5648e21",
        strip_prefix = "golangci-lint-1.29.0-darwin-amd64",
        urls = [
            "https://github.com/golangci/golangci-lint/releases/download/v1.29.0/golangci-lint-1.29.0-darwin-amd64.tar.gz",
        ],
        build_file = "//third_party:export.BUILD",
    )

    http_archive(
        name = "golangci-lint_linux_amd64",
        sha256 = "98b1eb7c74766079e1deebc3388c13db9bfa9fa0769046d786cf8d1553d7d68b",
        strip_prefix = "golangci-lint-1.29.0-linux-amd64",
        urls = [
            "https://github.com/golangci/golangci-lint/releases/download/v1.29.0/golangci-lint-1.29.0-linux-amd64.tar.gz",
        ],
        build_file = "//third_party:export.BUILD",
    )
