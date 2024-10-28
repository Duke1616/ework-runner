import json


class JsonBuilder:
    def __init__(self):
        # 初始化一个字典来存储 JSON 键值对
        self.data = {}

    def add_to_json(self, key, value):
        # 添加键值对到字典中
        self.data[key] = value

    def finalize_json(self):
        # 将字典转换为 JSON 字符串并返回
        return json.dumps(self.data, ensure_ascii=False)