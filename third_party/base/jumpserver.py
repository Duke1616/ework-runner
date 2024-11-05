from httpsig.requests_auth import HTTPSignatureAuth
import requests, datetime, json


signature_headers = ['(request-target)', 'accept', 'date']
gmt_form = '%a, %d %b %Y %H:%M:%S GMT'


class JumpServer:
    def __init__(self,  jms_url: str):
        self.auth = HTTPSignatureAuth
        self.jms_url = jms_url
        self.headers = {
            'Accept': 'application/json',
            'X-JMS-ORG': '00000000-0000-0000-0000-000000000002',
            'Date': datetime.datetime.utcnow().strftime(gmt_form)
        }

    def get_auth(self, KeyID, SecretID):
        self.auth = HTTPSignatureAuth(key_id=KeyID, secret=SecretID, algorithm='hmac-sha256', headers=signature_headers)

    def create_user(self, username: str, displayName: str, email: str):
        url = self.jms_url + '/api/v1/users/users/'
        data = {
            "source": "ldap",
            "username": username,
            "name": displayName,
            "email": email,
            "mfa_level": 0,
            "is_active": True
        }
        response = requests.post(url, auth=self.auth, headers=self.headers, data=data)
        if response.status_code == 201:
            return json.loads(response.text)
        else:
            return Exception(f"创建用户: {username}. 返回: {response.text}")

    def get_user(self, username):
        url = self.jms_url + '/api/v1/users/users/'
        parser = {
            "username": username
        }
        response = requests.get(url, auth=self.auth, headers=self.headers, params=parser)
        if response.status_code == 200:
            return json.loads(response.text)
        else:
            return Exception(f"查询用户: {username}. 返回: {response.text}")

    def create_system_user(self, username: str, displayName: str, cmd_filters: list):
        url = self.jms_url + '/api/v1/assets/system-users/'
        data = {
            "name": displayName,
            "username": username,
            "sudo": "/bin/whoami,/usr/bin/docker",
            "sftp_root": "/home/" + username,
            "home": "/home/" + username,
            "protocol": "ssh",
            "login_mode": "auto",
            "auto_push": True,
            "cmd_filters": cmd_filters
        }
        response = requests.post(url, auth=self.auth, headers=self.headers, data=data)
        if response.status_code == 201:
            return json.loads(response.text)
        else:
            return Exception(f"创建系统用户: {username}. 返回: {response.text}")

    def get_system_user(self, username: str):
        url = self.jms_url + '/api/v1/assets/system-users/'
        parser = {
            "username": username
        }
        response = requests.get(url, auth=self.auth, headers=self.headers, params=parser)
        if response.status_code == 200:
            return json.loads(response.text)
        else:
            return Exception(f"查询系统用户: {username}. 返回: {response.text}")

    def find_or_create_system_user(self, username: str, displayName: str, cmd_filters: list):
        user = self.get_system_user(username)
        if user:
            return user

        return self.create_system_user(username, displayName, cmd_filters)

    def find_or_create_user(self, username: str, displayName: str, email: str):
        user = self.get_user(username)
        if user:
            return user

        return self.create_user(username, displayName, email)

    def create_asset_authorization(self, username: str, displayName: str):
        url = self.jms_url + '/api/v1/perms/asset-permissions/'
        data = {
        }
        response = requests.post(url, auth=self.auth, headers=self.headers, data=data)
        print(json.loads(response.text))

    def put_asset_authorization(self, username: str, displayName: str):
        url = self.jms_url + '/api/v1/perms/asset-permissions/'
        data = {
        }
        response = requests.put(url, auth=self.auth, headers=self.headers, data=data)
        print(json.loads(response.text))

    def get_asset_authorization(self):
        url = self.jms_url + '/api/v1/perms/asset-permissions/'

    def get_user_info(self):
        url = self.jms_url + '/api/v1/users/users/'
        response = requests.get(url, auth=self.auth, headers=self.headers)
        print(json.loads(response.text))