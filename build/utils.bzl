# use genrule to specify platform http_file
def platform_genrule(*names, suffix = "file"):
    for name in names:
        native.genrule(
            name = name,
            srcs = select({
                "//build:macos": ["@%s_darwin_amd64//%s" % (name, suffix)],
                "//conditions:default": ["@%s_linux_amd64//%s" % (name, suffix)],
            }),
            outs = ["%s_file" % name],
            cmd = "cp $< $@",
            executable = True,
            visibility = ["//visibility:public"],
        )