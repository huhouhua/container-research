import os
from openai import OpenAI

client = OpenAI(
    # base_url="https://api.deepseek.com"
)
file_name = client.files.create(file=open("log.jsonl", "rb"), purpose="fine-tune")
file_id=file_name.id

print("File ID: ",file_id)

# 创建微调任务
finetune_job = client.fine_tuning.jobs.create(
    training_file=file_id, model="gpt-4o-mini-2024-07-18"
)
job_id = finetune_job.id
print("job_id is: ", job_id)