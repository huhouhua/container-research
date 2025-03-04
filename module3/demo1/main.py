
# pip install --upgrade openai
from openai import OpenAI

client = OpenAI(
 api_key="sk-06ba590654bc46b18ef0083ebcdf5f23",
    base_url="https://api.deepseek.com"
)

completion = client.chat.completions.create(
model="deepseek-chat",
    response_format={"type": "json_object"},
    messages=[
        {
            "role": "system",
            "content": '你现在是一个 JSON 对象提取专家，请参考我的 JSON 定义输出 JSON 对象。示例：{"service_name":"","action":""}，其中，action 可以是 get_log（获取日志）、restart（重启服务）、delete（删除工作负载）',
        },
        {
            "role": "user",
            "content": "帮我重启 payment 服务。",
        },
    ],
)

print(completion.choices[0].message.content)