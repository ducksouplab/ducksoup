This tutorial aims to guide users through the usage of DuckSoup in local in their own machine.
To do this, it guides users through the docker hub installation 

# Install Docker

If you are using Windows or Mac, install docker desktop:
https://docs.docker.com/desktop/setup/install/mac-install/ 

Then create an account in docker. 
Then open the new Docker Desktop app in your computer.
Then open the terminal and check that docker installation worked:
‘docker --version’

If you are using linux install docker following the instructions [here](https://docs.docker.com/engine/install/ubuntu/).

It might be useful for users to familiarize themselves with docker at this point. Docker is a lightweight containerization platform that encapsulates applications and all their dependencies into a single, portable container. It allows for consistent environments across development, testing, and production by isolating applications from the underlying system. This approach simplifies deployment, enhances scalability, and improves resource efficiency in modern software development. We share ducksoup in docker containers. The explanation of how to use them is below.

# Download and setup Ducksoup
Now that docker is installed, pull the latest version of the ducksoup docker image. To do this, paste this in the terminal.

In Intel Machines:
```docker pull ducksouplab/ducksoup:latest```

In Mac Silicon machines:
```
docker pull ducksouplab/ducksoup:arm_latest
docker tag ducksouplab/ducksoup:arm_latest ducksoup:latest
```

Test that the image works in your computer. 
To do this,create a new folder and cd into it:
```
mkdir ducksoup_test
cd ducksoup_test
```

Execute ducksoup:
```
docker run --name ducksoup_1 -u $(id -u arias):$(id -g arias) -p 8101:8100 -e DUCKSOUP_TEST_LOGIN=admin -e DUCKSOUP_TEST_PASSWORD=admin -e DUCKSOUP_NVCODEC=false -e DUCKSOUP_NVCUDA=false -e GST_DEBUG=3 -e DUCKSOUP_ALLOWED_WS_ORIGINS=http://localhost:8101 -e DUCKSOUP_JITTER_BUFFER=250 -e DUCKSOUP_GENERATE_PLOTS=true -e DUCKSOUP_GENERATE_TWCC=true -v $(pwd)/plugins:/app/plugins:ro -v $(pwd)/data:/app/data -v $(pwd)/log:/app/log --rm ducksoup:latest
```

Now, open the following address in your browser.
```
http://localhost:8101/test/mirror/ 
ID : admin 
Pswd : admin
```
You will be asked to enable the webcam use by your webcam, click allow.:


Now in the mirror web page click start. You should see and hear yourself twice (up and bottom of the page). This is Ducksoup Mirror mode, where a person can see themselves. You can add gstreamer plugins in the VideoFX and audioFX fields, to test different transformation plugins. For instance, try to put one of these in the VideoFx/AudioFx field and then click start:
VideoFX:
- agingtv
- fisheye

AudioFX:
- volume 0.5
- pitch 0.9
- webrtcdsp echo-cancel=false gain-control=false noise-suppression=true noise-suppression-level=high'

The complete list of available plugins and their parameters can be found online in the gstreamer [website](https://gstreamer.freedesktop.org/documentation/plugins_doc.html?gi-language=c).

Once you tested DuckSoup and their plugins, you can stop it by doing Ctrl + C in the terminal or by closing the terminal.  Then, to completely stop the container running the image execute:
```docker kill /ducksoup_1```

You can run the previous image with different parameters depending on your requirements. For instance, to reduce the latency reduce DUCKSOUP_JITTER_BUFFER— you can use use ```DUCKSOUP_JITTER_BUFFER=25``` for very low latency, just replace this parameter in the command line above. This unit is in milliseconds and tells ducksoup how much time to wait before sending the pipeline. If you are running in local, this can be reduced. All DuckSoup parameters are described in the [main read me documentation file](https://github.com/ducksouplab/ducksoup).

# Incorporate Mozza to perform real-time smile manipulation
If you want to perform real time smile manipulations follow the steps below.

To use our custom smile manipulation algorithm, we need to integrate into Ducksoup our custom Mozza plugin. To do this, make sure you are inside the ```ducksoup_test``` directory created above: 
```
cd ducksoup_test
```

Inside this directory, create a new folder called “plugins”:
```mkdir plugins```

Now, download the Mozza docker image.
In Mac:
```
docker pull ducksouplab/mozza:arm_latest
docker tag ducksouplab/mozza:arm_latest mozza:latest
```
In intel machines:
```
docker pull ducksouplab/mozza:latest
```

Now, copy the Mozza plugin and required files from that docker image. To do this, first, run the mozza image:
```docker run -d --name mozza_runner mozza:latest```

Now that the container is running, perform the following commands to copy the plugin and required files from mozza.
```
docker cp mozza_runner:gstmozza/build/libgstmozza.so plugins/libgstmozza.so
docker cp mozza_runner:gstmozza/build/lib/imgwarp/libimgwarp.so plugins/libimgwarp.so
docker cp mozza_runner:gstmozza/data/in/smile10.dfm plugins/smile10.dfm
```

Finally, download the `shape_predictor_68_face_landmarks.dat` model file. It can be found online at  this link : [http://dlib.net/files/shape_predictor_68_face_landmarks.dat.bz2](http://dlib.net/files/shape_predictor_68_face_landmarks.dat.bz2). Put the downloaded file inside the plugins/ folder.

Now, If you do: 
```ls plugins/```

You should see the files:
```
libgstmozza.so
libimgwarp.so
smile10.dfm
shape_predictor_68_face_landmarks.dat
```

Now, inside the ```ducksoup_test``` folder—which should have the “plugins” folder at this stage with all  the mozza files within it—, run ducksoup again.
```
docker run --name ducksoup_1 -u $(id -u arias):$(id -g arias) -p 8101:8100 -e DUCKSOUP_TEST_LOGIN=admin -e DUCKSOUP_TEST_PASSWORD=admin -e DUCKSOUP_NVCODEC=false -e DUCKSOUP_NVCUDA=false -e GST_DEBUG=3 -e DUCKSOUP_ALLOWED_WS_ORIGINS=http://localhost:8101 -e DUCKSOUP_JITTER_BUFFER=250 -e DUCKSOUP_GENERATE_PLOTS=true -e DUCKSOUP_GENERATE_TWCC=true -v $(pwd)/plugins:/app/plugins:ro -v $(pwd)/data:/app/data -v $(pwd)/log:/app/log --rm ducksoup:latest
```

Now, this running version of ducksoup has access to the mozza manipulation plugin for real time smile manipulation. To check this is the case, go to: http://localhost:8101/test/mirror/

Add ```mozza deform=smile10 alpha=1.1 beta=0.001 fc=1.0``` in the “Video FX” field and click start. You should see yourself with an increased smile. Change alpha to increase or decrease your smile. Alpga usually goes from -1 to 1 for realistic transformations. You can change in real-time it to see its effect by changing the fields in the mirror page as follows:
Property: alpha
Float: -0.9
Transition: 1000

Click send. You should see your changes visibly in your face.

 Check the [Mozza repository](https://github.com/ducksouplab/mozza) for more information about the parameters that can be used, as well as [this tutorial](https://github.com/ducksouplab/mozza/blob/main/tutorials/Use_mozza_in_local.md) if you want to use mozza offline to transform videos.

# Develop a new experiment using Ducksoup

We have another tutorial which aims to show you how to code your own experiment using DuckSoup. To do this, we have another repository, which runs python code based on otree to make the interface with DuckSoup. You can find a tutorial explaning how to do this here : https://github.com/ducksouplab/experiment_templates/blob/main/tutorial/tutorial.md











