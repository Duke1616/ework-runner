#!/bin/sh
# 脚本描述信息

# 工单传递传输
args=$1
vars=$2

source $vars
source ./third_party/utils/want_result.sh 

# 脚本主体
main() {
    # 脚本的主要逻辑
    echo $args
    echo $days

    # 示例：添加键值对
    add_to_json "key1" "value1"
    add_to_json "key2" "value2"
    add_to_json "key3" "value3"

    # 结束 JSON 字符串
    finalize_json

    # 打印构建的 JSON 字符串
    echo "$json"
}

main "$@"
