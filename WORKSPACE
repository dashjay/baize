# gazelle:repository_macro build/repos.bzl%go_repositories
# gazelle:repo bazel_gazelle
workspace(name = "com_github_dashjay_baize")

load("//build:deps.bzl", "deps")

deps()

load("@io_bazel_rules_go//go:deps.bzl", "go_download_sdk", "go_register_toolchains", "go_rules_dependencies")

go_download_sdk(
    name = "go_sdk",
    urls = ["https://golang.google.cn/dl/{}"],
    version = "1.18.1",
)

load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies")
load("//build:repos.bzl", "all_go_repositories")

all_go_repositories()

go_rules_dependencies()

go_register_toolchains()

gazelle_dependencies()

load("@com_github_bazelbuild_remote_apis//:repository_rules.bzl", "switched_rules_by_language")

switched_rules_by_language(
    name = "bazel_remote_apis_imports",
    go = True,
)
