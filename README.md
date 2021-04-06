# mailtohttp
mail to http

# installation
> Requirement: Golang Version >= 1.16.0
```
go install  github.com/VirgilZhao/mailtohttp
```
# useage
```
./mailtohttp -port 1323 -password test -encryptKey ABCDEFGHIGKLMNOP
``` 
> Params Description
> 
>-port : port number
>
>-password: login password
>
>-encryptKey: use AES encrypt config file, key must be 16 character length


open browser access http://127.0.0.1:1323 login to use

![image](https://github.com/VirgilZhao/mailtohttp/blob/main/images/login.PNG)
 

# config service
After login, you can config service by click "config service" button
![image](https://github.com/VirgilZhao/mailtohttp/blob/main/images/main.PNG)

first is email config, IMAP mail service supportted only currently, 'Folder' means your can specify subfolder like 'Inbox/facebook', then the service only read mails inside this folder.
![image](https://github.com/VirgilZhao/mailtohttp/blob/main/images/config_email.PNG)

content pattern config, you can add patterns here, use regex to match the content in mail, the return match value for a pattern is a list, because regex may return multi match content. 
![image](https://github.com/VirgilZhao/mailtohttp/blob/main/images/config_pattern.PNG)

config callback url, use http or https url to get the match result for each mail, the service will POST parttern result data to the url.
![image](https://github.com/VirgilZhao/mailtohttp/blob/main/images/config_http.PNG)

then click 'start service' button, enjoy!

callback request body format
```
{
  params:[
   {
     name: 'test pattern1',
     value: ['match1', 'match2']
   },
   {
     name: 'test pattern2',
     value: ['match1', 'match2']
   }
  ]
}
```
