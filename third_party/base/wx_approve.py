def parsing_data(data):
    extracted_data = {}
    for item in data['apply_data']['contents']:
        if item["control"] == "Selector":
            title = item['title'][0]['text']
            options = item['value']['selector']['options'] \
                if 'selector' in item['value'] and 'options' in item['value']['selector'] else []
            extracted_data[title] = [option['value'][0]['text'] for option in options]
    return extracted_data
