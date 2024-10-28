# 初始化 JSON 字符串
json="{"

# 函数：添加键值对到 JSON 对象
add_to_json() {
    key="$1"
    value="$2"

    # 添加逗号分隔符
    if [ "$json" != "{" ]; then
        json+=","
    fi

    # 将键值对格式化为 JSON，确保值用双引号包裹
    json+="\"$key\":\"$value\""
}

# 函数：结束 JSON 字符串
finalize_json() {
    json+="}"
}